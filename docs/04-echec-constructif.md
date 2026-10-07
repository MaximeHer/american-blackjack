# Échec constructif : la mémoïsation qui ralentit tout

Documentation d'une tentative d'optimisation **contre-productive**, mesurée et
expliquée mécaniquement avant retour arrière.

Ce document répond au **critère 4**. Le code correspondant vit sur la branche
`echec/memoisation-total`, délibérément **conservée et non fusionnée** : une
branche d'échec préservée est une pièce à conviction.

---

## 1. Le raisonnement, qui paraissait solide

Le binaire instrumenté mesure **10,38 appels à `Hand.Total()` par coup**.

Beaucoup portent sur la même main sans qu'elle ait changé : la stratégie
l'interroge pour choisir, le test de dépassement pour savoir si le joueur a
sauté, la comparaison finale pour le règlement. Trois lectures du même état.

L'idée s'impose donc d'elle-même : **mettre le résultat en cache**, indexé par la
main. C'est le « Mémoïsation & Caching » du compromis espace-temps du cours —
dépenser de la mémoire pour économiser du CPU.

```go
var totalMemo = map[string][2]int{}

func (h *Hand) Total() (int, bool) {
    key := make([]byte, len(h.Cards))
    for i, c := range h.Cards {
        key[i] = byte(c)
    }
    if v, ok := totalMemo[string(key)]; ok {
        return v[0], v[1] == 1
    }
    // … calcul …
    totalMemo[string(key)] = [2]int{total, soft}
    return total, soft
}
```

---

## 2. Le résultat

| Mesure | Sans cache | Avec cache | Écart |
|---|---|---|---|
| `HandTotal` | 3,687 ns | **27,65 ns** | **×7,5 plus lent** |
| `PlayRound` | 255,9 ns | 926,3 ns | **+261,9 %** |
| Débit | 4 210 560 coups/s | **1 151 994** | **−72,6 %** |
| Temps par coup | 237 ns | 868 ns | ×3,66 |

**L'optimisation a divisé le débit par 3,7.**

---

## 3. Explication mécanique

Deux causes distinctes, toutes deux mesurées.

### 3.1 Le cache coûte plus cher que le calcul qu'il évite

Consulter la map exige quatre opérations :

1. construire une clé depuis les cartes de la main ;
2. **hacher** cette clé ;
3. sonder le seau correspondant et comparer les hauts de hash ;
4. comparer la clé complète en cas de correspondance.

Total mesuré : **27,65 ns**.

Le calcul évité, lui, parcourt deux à trois cartes et lit leur valeur dans un
tableau indexé. Depuis le rang 2 — la carte compactée sur un octet — il coûte
**3,687 ns**.

> **Le cache est 7,5 fois plus cher que ce qu'il remplace.**

### 3.2 Le cache retient tout, et le ramasse-miettes le paie

La clé est la **suite ordonnée** des cartes de la main. L'espace des clés est
donc immense — pour des mains de deux à cinq cartes tirées parmi 52, il se
compte en centaines de milliers de combinaisons — et **aucune entrée n'est
jamais éliminée**.

| | Sans cache | Avec cache | Écart |
|---|---|---|---|
| Objets vivants sur le tas | 22 097 | **387 523** | **×17,5** |
| Part CPU du ramasse-miettes | 4,29 % | **11,31 %** | ×2,6 |
| Alloué par coup | 72 o | 120 o | +67 % |

Chaque cycle de ramassage doit désormais **tracer 387 000 objets vivants
supplémentaires**, pour un cache dont rien n'est jamais purgé. C'est exactement
le phénomène que l'énoncé désigne par « cache augmentant la pression du GC ».

Croissance mesurée du tas selon la durée d'exécution :

| Coups simulés | Objets sur le tas |
|---|---|
| 200 000 | 243 879 |
| 1 000 000 | 818 902 |
| 2 000 000 | 862 216 |

---

## 4. L'enseignement, et il est contre-intuitif

Le compromis espace-temps joue **dans les deux sens**. Mettre en cache n'est
rentable que si le calcul évité coûte plus cher que la consultation.

Or voici le point remarquable : **c'est notre propre optimisation précédente qui
a rendu celle-ci perdante.**

Sur la version de référence, `Hand.Total` coûtait **31,27 ns** — la carte était
décrite par deux chaînes et sa valeur se lisait dans une `map[string]int`. À ce
prix-là, un cache à 27,65 ns aurait été **un gain**.

Le rang 2 a fait tomber `Total` à 3,687 ns. La même idée, inchangée, est passée
de bénéfique à catastrophique.

> **Une idée d'optimisation n'est ni bonne ni mauvaise dans l'absolu. Elle l'est
> relativement à l'état du code.**

C'est aussi la justification empirique de la règle « reprofiler entre les
paliers » : chaque optimisation ne déplace pas seulement le goulot, elle change
la rentabilité de toutes celles qui restent à faire.

---

## 5. Pourquoi cet échec était indétectable autrement

La correction est **intégralement préservée** : l'avantage de la maison reste à
+0,4144 %, et `TestHandTotal` passe sur tous ses cas, y compris les mains à deux
et trois As.

L'échec est **purement de performance**. Aucun test fonctionnel ne l'aurait
signalé, aucune revue de code ne l'aurait rejeté — le raisonnement initial est
parfaitement défendable à la lecture.

Seule la mesure l'a révélé. C'est précisément ce que le cours formule par
« optimiser après mesure » et « on ne devine jamais le hot path, on le mesure ».

---

## 6. Deux autres échecs identifiés, non encore mesurés

Par honnêteté : seul le premier a été conduit jusqu'à la mesure.

**Parallélisation prématurée.** Lancer une goroutine par coup plutôt qu'un worker
pool par sabot. Le coût de création et d'ordonnancement d'une goroutine est du
même ordre que le travail utile d'un coup — désormais 241 ns — donc le
changement de contexte doit dominer. À mesurer lors de la phase de concurrence.

**Générateur pseudo-aléatoire global partagé.** Les fonctions de paquet de
`math/rand` sont protégées par un verrou global. Appelées depuis N goroutines,
elles sérialisent intégralement l'accès et la version parallèle peut devenir plus
lente que la séquentielle. C'est l'échec le plus instructif des trois, car il est
involontaire : un développeur écrit `rand.Intn()` sans savoir qu'il prend un
verrou.

---

## 7. Reproduire

```bash
git checkout echec/memoisation-total
go test -run '^$' -bench 'HandTotal|PlayRound$' -benchmem -benchtime 2s -count 6 ./internal/blackjack
go run ./cmd/simulate -rounds 1000000      # observer « Part CPU du GC » et « Objets sur le tas »
git checkout main
```
