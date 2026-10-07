# Perspectives d'évolution backend

Plan de travail des optimisations du moteur, de la version de référence vers la
version finale.

Ce document est une **feuille de route, pas un journal de résultats**. Tout
chiffre annoncé ici est une *hypothèse à vérifier*, jamais un acquis. Les
mesures réelles s'inscrivent au fur et à mesure dans le journal d'optimisation
du rapport d'audit.

---

## 0. Méthode

### Format imposé

Chaque palier est formulé selon les directives de
[`constitution.md`](../constitution.md) : une hypothèse portant sur un effet
physique du matériel, la commande exacte qui la vérifie, et le résultat qui
l'invaliderait. Une proposition sans ces trois volets n'est pas recevable.

### Protocole de mesure

```bash
# Avant toute modification
go test -run '^$' -bench . -benchmem -count 10 ./internal/blackjack > bench/avant.txt

# Une seule optimisation, un seul commit
# ...

go test -run '^$' -bench . -benchmem -count 10 ./internal/blackjack > bench/apres.txt
benchstat bench/avant.txt bench/apres.txt
```

Trois règles non négociables :

1. **Un changement à la fois.** Deux optimisations dans un commit rendent
   l'attribution du gain impossible, et le journal d'optimisation perd toute
   valeur probante.
2. **Comparer par `benchstat`, jamais deux nombres bruts.** Un écart inférieur
   à la dispersion des mesures n'est pas un gain.
3. **Rejeter toute mesure dont la variance dépasse 5 %.** Relancer après avoir
   fermé les processus parasites.

### Invariant absolu

```bash
go test ./...
```

L'avantage de la maison mesuré ne doit pas bouger au-delà du bruit
statistique. `TestHouseEdgeOracle` borne l'écart à 4 erreurs-types de la valeur
publiée, calculées depuis la variance réellement observée.

**Une optimisation qui casse l'oracle est annulée.** Sans discussion, sans
tentative de rattrapage. Optimiser en cassant la correction n'est pas
optimiser.

Un cas mérite une précision : plusieurs paliers **modifient l'ordre de
consommation du générateur pseudo-aléatoire** — le passage à Fisher-Yates en
premier lieu. À graine égale, les cartes sortiront donc dans un ordre
différent, et l'avantage mesuré changera de quelques centièmes. **C'est
légitime** : l'oracle est statistique, pas bit-à-bit. Ce qui doit rester
constant, c'est la convergence vers la valeur publiée, pas la séquence exacte.
Le déterminisme à graine fixée, lui, doit être préservé sans exception.

### État de départ, mesuré

| Mesure | Valeur de référence |
|---|---|
| Débit de bout en bout | 342 306 coups/s médians — 2 921 ns par coup (écart-type 4,52 % sur 15 exécutions) |
| `PlayRound` | 5 200 ns/op, 1 656 o/op, **46 allocs/op** |
| `PlayRound` + paris annexes | 4 900 ns/op, 2 081 o/op, **57 allocs/op** |
| `Shuffle` | 19 300 ns/op, 15 488 o/op, **220 allocs/op** |
| `Decide` | 264 ns/op, 48 o/op, 3 allocs/op |
| `HandTotal` | 36,6 ns/op, 0 alloc |
| `Simulate` 10⁴ coups | 41 ms/op, **16,5 Mo/op**, **469 801 allocs/op** |
| `Card` / `Hand` / `RoundResult` | 32 o / 56 o (19 o de remplissage) / 104 o (28 o) |

---

## Phase A — Supprimer le travail inutile

Trois paliers indépendants, sans refonte de structure. Ils donnent les premiers
gains et valident le protocole de mesure avant d'attaquer le dur.

### Palier 1 — Rendre le journal narratif explicite

**État.** `PlayRound` construit une dizaine de lignes de récit par coup, par
`fmt.Sprintf`, y compris dans la boucle de simulation qui ne les lit jamais.

Ce n'est pas du travail gratuit : le journal alimente le panneau de narration
de l'interface, via `/api/sample-round`. Le défaut est de le produire **sans
que l'appelant l'ait demandé**. C'est la forme la plus répandue de gaspillage
en production — un code partagé entre deux usages paie le coût du plus
exigeant des deux.

```
HYPOTHÈSE    : fmt.Sprintf analyse sa chaîne de format à l'exécution, alloue
               un []byte puis le convertit en string. Chaque ligne produit un
               objet immédiatement mort, qui alimente le ramasse-miettes sans
               être lu. Retirer le journal du chemin de simulation devrait
               supprimer ~10 des 46 allocations par coup et une part
               substantielle des 1 656 octets.
VÉRIFICATION : go test -run '^$' -bench PlayRound -benchmem -count 10 \
                 ./internal/blackjack > bench/apres.txt
               benchstat bench/baseline.txt bench/apres.txt
RÉFUTATION   : un gain inférieur à 15 % sur ns/op signifie que le formatage
               n'était pas le poste dominant et que le profil CPU doit être
               relu avant d'aller plus loin.
```

**Conception.** Passer un collecteur optionnel plutôt qu'un drapeau booléen
dispersé : `PlayRound(s, r, bet, sb, trace *Trace)` où `trace == nil` en
simulation. Le test de nullité est unique par événement et parfaitement prédit
par le processeur. L'interface, elle, passe un collecteur non nul et conserve
sa fonctionnalité intacte.

**Axe de la grille.** Zero-allocation, pression du ramasse-miettes.

---

### Palier 2 — Tables de gains des paris annexes en tableaux

**État.** `perfectPairsPaytable`, `twentyOnePlus3Paytable` et
`luckyLadiesPaytable` sont des `map[string]float64` consultées sur le hot path,
et les libellés de combinaison sont des chaînes.

```
HYPOTHÈSE    : le domaine est borné et connu à la compilation — cinq
               combinaisons au maximum par pari. Une énumération entière
               indexant un tableau de gains supprime le hachage de chaîne et
               la comparaison de libellés. Gain attendu sur
               BenchmarkSideBetEval (105 ns/op) et sur
               BenchmarkPlayRoundSideBets (4 900 ns/op).
VÉRIFICATION : go test -run '^$' -bench 'SideBet|PlayRoundSideBets' -benchmem \
                 -count 10 ./internal/blackjack
RÉFUTATION   : aucun gain mesurable signifie que le compilateur avait déjà
               optimisé ces accès, ou que l'évaluation des paris annexes est
               négligeable devant le reste du coup.
```

**Axe de la grille.** `map` comme table de correspondance à domaine borné.

---

### Palier 3 — Dévirtualiser la stratégie

**État.** `DefaultStrategy` est une variable de type `Strategy`, une interface.
Chaque décision passe donc par une table de méthodes.

```
HYPOTHÈSE    : l'appel dynamique empêche l'inlining de Decide, donc la
               propagation de constantes et l'élimination des branches mortes.
               Remplacer la variable d'interface par un type concret doit
               permettre au compilateur d'inliner et de spécialiser. Effet
               attendu modeste en soi — quelques pourcents — mais il débloque
               les optimisations des paliers suivants sur le même chemin.
VÉRIFICATION : go test -run '^$' -bench Decide -benchmem -count 10 \
                 ./internal/blackjack
               go build -gcflags='-m' ./internal/blackjack 2>&1 | grep -i 'inlin'
RÉFUTATION   : si le rapport d'inlining ne change pas, le compilateur avait
               déjà dévirtualisé l'appel (il le fait quand une interface n'a
               qu'une implémentation visible) et l'hypothèse est fausse.
```

**Remarque honnête.** C'est le palier dont le gain est le plus incertain : Go
sait parfois dévirtualiser seul. S'il ne donne rien, **cela se documente
comme tel** — un palier neutre mesuré est une information, pas un échec.

**Axe de la grille.** Appel dynamique sur le hot path.

---

## Phase B — Représentation des données

Le cœur du travail. Le palier 5 conditionne presque tous les autres : il faut
le faire tôt, mais après avoir mesuré la phase A pour ne pas mélanger les
effets.

### Palier 4 — `map[string]int` → `switch` pour la valeur des cartes

**État.** `Card.Value()` interroge `cardValues`, une `map[string]int`. Or
`Hand.Total()` parcourt toutes les cartes et est appelé dix à quinze fois par
coup : on paie donc une cinquantaine de hachages de chaîne par coup.

```
HYPOTHÈSE    : remplacer la map par un switch supprime le calcul de hachage,
               mais conserve la comparaison de chaînes. Le gain doit donc être
               réel mais partiel — de l'ordre de la moitié du coût de
               BenchmarkHandTotal (36,6 ns pour 3 cartes, soit ~12 ns par
               carte). Le reste ne tombera qu'avec un rang numérique
               (palier 5).
VÉRIFICATION : go test -run '^$' -bench HandTotal -benchmem -count 10 \
                 ./internal/blackjack
RÉFUTATION   : un gain supérieur à 80 % invaliderait l'analyse : cela voudrait
               dire que le hachage, et non la comparaison de chaînes,
               constituait l'essentiel du coût.
```

**Pourquoi ce palier intermédiaire.** Il serait tentant de passer directement
au rang numérique. Le mesurer séparément a une valeur pédagogique propre :
il montre qu'une optimisation évidente peut ne donner qu'une fraction du gain
espéré, parce que le vrai coût était ailleurs. C'est exactement le genre de
constat que le critère 3 attend.

**Axe de la grille.** `map` comme table de correspondance.

---

### Palier 5 — Carte compactée sur un octet

**Le palier structurant.** Il débloque les paliers 6 à 12.

**État.** `Card` est une structure de deux `string`, soit **32 octets** : deux
en-têtes de 16 octets (pointeur + longueur). Une ligne de cache de 64 octets
contient donc **2 cartes**.

```
HYPOTHÈSE    : une carte se décrit entièrement par 4 bits de rang (13 valeurs)
               et 2 bits d'enseigne (4 valeurs), soit un seul octet. Le
               facteur sur la représentation est de 32, et une ligne de cache
               passe de 2 à 64 cartes. Un sabot de 4 jeux passe de 6 656 o
               (104 lignes de cache) à 208 o (4 lignes) : il devient
               intégralement résident en L1. Les défauts de cache sur Deal et
               Total doivent chuter de plus d'un ordre de grandeur.
VÉRIFICATION : go test -run '^$' -bench . -benchmem -count 10 \
                 ./internal/blackjack > bench/apres.txt
               benchstat bench/baseline.txt bench/apres.txt
               go test -run TestStructSizes -v ./internal/blackjack
RÉFUTATION   : un gain inférieur à 20 % sur BenchmarkPlayRound signifierait
               que le moteur n'était pas limité par la mémoire mais par le
               calcul, et réorienterait tout le plan vers la réduction du
               travail plutôt que vers la localité.
```

**Conception.** `type Card uint8`, avec `card = rank<<2 | suit`. Le rang et
l'enseigne s'extraient par décalage et masque — deux instructions, sans accès
mémoire. La valeur au blackjack se lit dans un tableau de 13 entrées indexé par
le rang : plus de hachage, plus de comparaison de chaînes.

Les libellés lisibles (`"Dame de Coeur"`) restent nécessaires pour l'interface
et le journal : ils deviennent une conversion explicite, appelée uniquement
hors du chemin mesuré.

**Axe de la grille.** Lignes de cache 64 o, localité, struct padding.

---

### Palier 6 — `[]*Card` → `[]Card`

**État.** Le sabot et les mains sont des slices de **pointeurs** vers des
cartes allouées une par une. Parcourir une main déréférence autant de pointeurs
qu'elle a de cartes, vers des adresses sans rapport entre elles.

```
HYPOTHÈSE    : stocker les cartes par valeur supprime une allocation par carte
               — 208 par rebattage — et rétablit la contiguïté. Le
               préchargeur matériel, inopérant sur une chaîne de pointeurs
               dispersés, redevient efficace sur un parcours séquentiel. Effet
               attendu sur les 220 allocations de Shuffle et sur les 46
               de PlayRound.
VÉRIFICATION : go test -run '^$' -bench 'Shuffle|Deal|PlayRound' -benchmem \
                 -count 10 ./internal/blackjack
RÉFUTATION   : si le nombre d'allocations par coup ne diminue pas, c'est que
               l'analyse d'échappement plaçait déjà ces cartes sur la pile, et
               l'hypothèse sur le tas était fausse.
```

**Dépendance.** Beaucoup plus efficace après le palier 5 : copier une carte
d'un octet est gratuit, copier 32 octets l'est moins.

**Axe de la grille.** Localité de cache, zero-allocation.

---

### Palier 7 — Sabot en tableau fixe distribué par curseur

**État.** `Shuffle` reconstruit le sabot par `append` — 15 488 octets et 220
allocations — et `Deal` retire la carte de tête par re-slicing, ce qui interdit
de réutiliser le tableau sous-jacent.

```
HYPOTHÈSE    : un tableau de taille fixe alloué une fois, plus un curseur
               d'index, supprime toute allocation au rebattage et rend l'accès
               strictement séquentiel et croissant — le motif que le
               préchargeur matériel détecte le mieux. BenchmarkShuffle doit
               passer à 0 allocs/op, et BenchmarkDeal également.
VÉRIFICATION : go test -run '^$' -bench 'Shuffle|Deal' -benchmem -count 10 \
                 ./internal/blackjack
RÉFUTATION   : des allocations résiduelles signalent que le tableau échappe
               vers le tas ; le vérifier par
               go build -gcflags='-m' ./internal/blackjack | grep escape
```

**Axe de la grille.** Zero-allocation, accès séquentiel.

---

### Palier 8 — Mélange de Fisher-Yates en place

**État mesuré.** Le mélange tire une carte au hasard dans le paquet et la
retire. L'algorithme est correct et uniforme, mais chaque retrait décale la fin
du slice.

L'index tiré étant uniforme, le nombre moyen de déplacements vaut n(n−1)/4,
soit **10 764** pour n = 208. Le comptage instrumenté en mesure **10 756** : la
prédiction théorique est confirmée. Fisher-Yates accomplit le même travail en
208 échanges sur place, soit un **facteur 52**.

```
HYPOTHÈSE    : la complexité passe de O(n²) à O(n), avec n = 208, soit un
               facteur 52 sur le nombre de déplacements (10 756 -> 208, mesuré
               par le binaire instrumenté). Le coût du mélange doit chuter de
               plus d'un ordre de grandeur, depuis 19 300 ns/op. Le rebattage
               survenant tous les ~29 coups, cela représente environ 660 ns par
               coup.
VÉRIFICATION : go test -run '^$' -bench Shuffle -benchmem -count 10 \
                 ./internal/blackjack
               go test -run 'TestShoeComposition|TestDeterminisme' -v \
                 ./internal/blackjack
RÉFUTATION   : un gain inférieur à un facteur 5 indiquerait que le coût était
               dominé par les allocations (palier 7) et non par le décalage
               quadratique.
```

**Attention.** Ce palier **change l'ordre des cartes à graine égale**.
`TestShoeComposition` doit continuer à prouver l'uniformité (4 exemplaires de
chacune des 52 cartes), et `TestDeterminisme` le déterminisme. L'avantage
mesuré bougera de quelques centièmes : c'est attendu et à documenter.

**Axe de la grille.** Complexité algorithmique, `memmove`.

---

### Palier 9 — Total de la main maintenu en incrémental

**État.** `Hand.Total()` recalcule tout depuis le début à chaque appel, et il
est appelé plusieurs fois par décision — stratégie, test de dépassement,
comparaison finale.

```
HYPOTHÈSE    : le total et le nombre d'As peuvent être maintenus à l'ajout
               d'une carte, dans deux champs de deux octets. Total() devient
               une lecture et un ajustement, en temps constant au lieu d'un
               parcours. BenchmarkHandTotal doit tomber sous les 5 ns/op.
VÉRIFICATION : go test -run '^$' -bench 'HandTotal|PlayRound' -benchmem \
                 -count 10 ./internal/blackjack
               go test -run TestHandTotal -v ./internal/blackjack
RÉFUTATION   : si PlayRound ne bouge pas alors que HandTotal s'améliore, c'est
               que Total() était appelé moins souvent que supposé — le
               vérifier au profil CPU avant de conclure.
```

**Risque de correction.** La réduction des As de 11 à 1 doit rester exacte sur
les mains à plusieurs As. `TestHandTotal` couvre déjà deux et trois As.

**Axe de la grille.** Élimination de travail redondant.

---

### Palier 10 — Réordonnancement des champs de structures

**État mesuré.** `Hand` occupe 56 octets pour 37 octets de champs utiles, soit
**19 octets de remplissage (34 %)**. `RoundResult` occupe 104 octets pour 76
utiles, soit **28 octets (27 %)**.

```
HYPOTHÈSE    : Go ne réordonne jamais les champs d'une structure. Les booléens
               intercalés entre les champs de 8 octets forcent donc un
               alignement coûteux. Les regrouper en fin de structure doit
               ramener Hand sous 40 octets et RoundResult sous 80, soit une
               structure de plus par ligne de cache.
VÉRIFICATION : go test -run TestStructSizes -v ./internal/blackjack
               go vet -vettool=$(which fieldalignment) ./internal/blackjack
RÉFUTATION   : un gain de taille sans gain de temps est possible et doit être
               reconnu comme tel : la structure n'était pas le goulot. Le
               palier reste justifié sur l'empreinte mémoire, pas sur le
               débit.
```

**Honnêteté requise.** C'est le palier le plus susceptible de donner un gain de
taille sans gain de vitesse. Le documenter ainsi vaut mieux que de lui
attribuer un gain qu'il n'a pas produit.

**Axe de la grille.** Struct padding, alignement des champs.

---

### Palier 11 — Mains en tableaux fixes

**État.** Les mains sont des slices alimentés par `append`.

```
HYPOTHÈSE    : une main de blackjack ne peut excéder 21 cartes (vingt-et-un as
               comptés 1, cas théorique). Un tableau fixe de 22 octets plus un
               compteur supprime le dernier append du hot path et permet à la
               main de rester sur la pile. Objectif : PlayRound à 0 allocs/op.
VÉRIFICATION : go test -run '^$' -bench PlayRound -benchmem -count 10 \
                 ./internal/blackjack
               go build -gcflags='-m' ./internal/blackjack 2>&1 | grep -i escape
RÉFUTATION   : si des allocations subsistent, identifier ce qui échappe encore
               — très probablement le slice de mains du coup, qui grandit aux
               séparations et justifie alors un sync.Pool ou un tableau fixe
               de 4 mains.
```

**Axe de la grille.** Zero-allocation.

---

### Palier 12 — Table de stratégie plate indexée arithmétiquement

**État.** Chaque décision construit une clé par `fmt.Sprintf`
(`"hard-16-10"`), puis hache la chaîne pour interroger une map : **3
allocations et 264 ns par décision**.

```
HYPOTHÈSE    : l'état de décision est entièrement décrit par trois entiers
               bornés — type de main (3), total (5 à 21), carte visible (10).
               Un tableau plat indexé par (kind*17 + total-5)*10 + up supprime
               le formatage, l'allocation et le hachage, au prix d'une
               multiplication et d'une addition. La table complète pèse moins
               de 600 octets et tient dans 10 lignes de cache. Decide doit
               passer sous 10 ns/op et 0 alloc.
VÉRIFICATION : go test -run '^$' -bench 'Decide|PlayRound' -benchmem \
                 -count 10 ./internal/blackjack
               go test -run TestTableCrossValidation -v ./internal/blackjack
RÉFUTATION   : un gain inférieur à un facteur 10 sur Decide signalerait que le
               coût venait de la logique de dégradation des décisions, et non
               de la consultation de la table.
```

**Garde-fou de correction.** La table plate doit être **générée depuis les
mêmes lignes sources** que la table actuelle, pas recopiée à la main. Une
recopie manuelle est la façon la plus sûre d'introduire une erreur silencieuse
dans la stratégie, qui dégraderait l'avantage mesuré sans provoquer d'erreur
visible. `/api/strategy` doit continuer à exposer la grille lue depuis le code.

**Axe de la grille.** `fmt.Sprintf` sur le hot path, `map` à domaine borné,
localité.

---

## Phase C — Concurrence et scalabilité

La machine dispose de **12 coeurs logiques, dont un seul est utilisé**. C'est
le gisement de gain le plus important en valeur absolue, et le plus piégeux.

### Palier 13 — Worker pool borné aux coeurs physiques

**État.** `Simulate` est une boucle strictement séquentielle.

```
HYPOTHÈSE    : les sabots sont indépendants — aucun état partagé entre deux
               séquences de coups. Le problème est donc massivement
               parallélisable, et l'accélération doit être quasi linéaire
               jusqu'au nombre de coeurs physiques, puis marginale sur les
               coeurs logiques (l'hyperthreading ne double pas les unités de
               calcul). Attendu : facteur 6 à 8 sur 12 coeurs logiques.
VÉRIFICATION : go test -run '^$' -bench Simulate -benchmem -count 10 \
                 -cpu 1,2,4,6,8,12 ./internal/blackjack
RÉFUTATION   : une accélération plafonnant sous 3 révèle une contention ou un
               faux partage de ligne de cache ; la localiser par
               go test -bench Simulate -mutexprofile=mutex.out puis
               go tool pprof -top mutex.out
```

**Dimensionnement.** Borner à `runtime.NumCPU()`, et **mesurer** la courbe
d'accélération plutôt que de la supposer. Le point d'inflexion entre coeurs
physiques et logiques est une mesure intéressante en soi.

**Axe de la grille.** Worker pool dimensionné aux coeurs physiques.

---

### Palier 14 — Générateur par worker, déterminisme préservé

**Le palier le plus subtil du projet.**

**État.** `Simulate` crée un `rand.Rand` unique. Partagé entre goroutines, il
devient un point de sérialisation ; et même sans contention, l'ordre
d'exécution non déterministe des workers détruirait la reproductibilité.

```
HYPOTHÈSE    : un générateur par worker, dont la graine est DÉRIVÉE de la
               graine maître par une fonction déterministe (seed_i =
               hash(master, i)), supprime tout partage et conserve une
               reproductibilité stricte : chaque worker produit exactement la
               même séquence à chaque exécution, indépendamment de l'ordre
               d'ordonnancement. L'agrégation finale doit être associative et
               indépendante de l'ordre d'arrivée.
VÉRIFICATION : go test -run TestDeterminisme -count 20 ./internal/blackjack
               go test -race -run TestDeterminisme ./internal/blackjack
RÉFUTATION   : deux exécutions de même graine donnant des statistiques
               différentes invalident la dérivation : l'agrégation dépend
               alors de l'ordre, ou un état est partagé par inadvertance.
```

**Point de méthode pour le rapport.** Le déterminisme sous parallélisme n'est
pas un détail d'implémentation : c'est la condition de validité du protocole de
mesure du critère 1. Une simulation parallèle non reproductible ne prouve rien.

**Axe de la grille.** Concurrence, reproductibilité.

---

### Palier 15 — Agrégation sans contention

```
HYPOTHÈSE    : agréger dans une structure partagée protégée par un verrou
               sérialise les workers. Accumuler localement puis fusionner une
               seule fois en fin de tâche supprime la contention. Si une
               agrégation incrémentale est nécessaire, sync/atomic sur des
               compteurs alignés et séparés par au moins 64 octets évite le
               faux partage de ligne de cache.
VÉRIFICATION : go test -run '^$' -bench Simulate -mutexprofile=mutex.out \
                 ./internal/blackjack
               go tool pprof -top mutex.out
RÉFUTATION   : un profil de contention déjà vide avant modification signifie
               que la fusion locale était déjà en place et que le palier est
               sans objet.
```

**Axe de la grille.** Synchronisation atomique, faux partage.

---

### Palier 16 — Arrêt précoce sur intervalle de confiance

**L'optimisation la plus intelligente du lot**, parce qu'elle ne réduit pas le
coût d'un coup : elle supprime des coups devenus inutiles.

```
HYPOTHÈSE    : l'erreur-type décroît en 1/racine(n). Une fois la précision
               demandée atteinte — par exemple ±0,01 point — tout coup
               supplémentaire est du calcul perdu. Surveiller l'erreur-type
               courante et arrêter dès le seuil franchi réduit le travail
               total sans dégrader la qualité du résultat. L'arrêt doit être
               coopératif et vérifié par lots, non à chaque coup, pour ne pas
               introduire de synchronisation sur le hot path.
VÉRIFICATION : go run ./cmd/simulate -precision 0.01 -quiet
               comparer le nombre de coups effectivement joués à la prévision
               théorique sigma²/epsilon² ~ 1,3 x 10^8
RÉFUTATION   : un arrêt systématiquement très au-delà ou très en-deçà de la
               prévision théorique révèle une erreur dans le calcul de
               l'erreur-type courante.
```

**Attention au déterminisme.** Un arrêt dépendant de l'ordre d'arrivée des
lots casserait la reproductibilité. L'arrêt doit se décider sur un nombre
entier de lots complets, dans un ordre fixé.

**Axe de la grille.** Arrêt précoce (*early cancellation*).

---

## Phase D — I/O réseau et persistance

**Reconnaissance d'une faiblesse du sujet.** Un simulateur n'a pas besoin
intrinsèquement d'une base de données. L'angle retenu est donc celui du
**cache de résultats précalculés par jeu de règles**, qui est défendable et
réellement utile : une configuration déjà simulée à la précision demandée n'a
aucune raison d'être recalculée.

### Palier 17 — API binaire Protobuf/gRPC

```
HYPOTHÈSE    : une demande de simulation se décrit par une poignée de champs
               numériques — nombre de jeux, drapeaux de règles, graine,
               précision. Encodée en Protobuf elle occupe quelques dizaines
               d'octets, contre plusieurs centaines en JSON, et son décodage
               n'exige ni analyse lexicale ni allocation de map. Le gain
               attendu porte sur la latence par requête et sur le débit du
               serveur sous charge.
VÉRIFICATION : hyperfine --warmup 3 --runs 50 \
                 'grpcurl -d @ ... Simulate' 'curl -s -XPOST .../api/simulate'
               go test -run '^$' -bench Codec -benchmem ./internal/transport
RÉFUTATION   : un écart négligeable signifierait que la sérialisation est
               dominée par le temps de simulation lui-même — ce qui est
               probable sur de grosses requêtes, et doit alors être dit : le
               gain ne vaut que sur des requêtes nombreuses et courtes.
```

**Axe de la grille.** Formats binaires haute performance.

---

### Palier 18 — Persistance indexée et cache LRU

```
HYPOTHÈSE    : les résultats indexés par (règles, graine, précision) se
               retrouvent en temps logarithmique avec un index composite,
               contre un balayage séquentiel sans index. EXPLAIN ANALYZE doit
               montrer le passage d'un Seq Scan à un Index Scan. Un cache LRU
               en mémoire devant la base évite l'aller-retour pour les
               configurations consultées en rafale.
VÉRIFICATION : EXPLAIN ANALYZE SELECT ... WHERE rules_hash = $1 AND seed = $2;
               avant et après CREATE INDEX
               hyperfine --warmup 3 'curl -s .../api/simulate?cached=true'
RÉFUTATION   : sur une table de quelques milliers de lignes, le planificateur
               peut légitimement préférer un balayage séquentiel. Il faudra
               alors mesurer sur un volume réaliste avant de conclure, et
               reconnaître que l'index n'apporte rien à petite échelle.
```

**Axe de la grille.** Indexation SQL, `EXPLAIN ANALYZE`, mise en cache.

---

## Échecs attendus — critère 4

Ces tentatives sont **planifiées pour échouer**. Chacune vit sur une branche
conservée et non fusionnée : une branche d'échec préservée est une pièce à
conviction.

### Échec A — Une goroutine par coup

```
MÉCANISME    : la création d'une goroutine et son ordonnancement coûtent de
               l'ordre de quelques centaines de nanosecondes à quelques
               microsecondes, du même ordre que le travail utile d'un coup.
               Le changement de contexte et la pression sur l'ordonnanceur
               doivent dominer, jusqu'à rendre la version parallèle plus lente
               que la séquentielle.
MESURE       : go test -bench Simulate -trace trace.out puis
               go tool trace trace.out — observer la part d'ordonnancement
ATTENDU      : régression nette, d'autant plus marquée que le grain est fin
```

C'est l'exemple de « parallélisation prématurée détruite par le
context-switching » cité dans l'énoncé.

### Échec B — Générateur pseudo-aléatoire global partagé

**L'échec le plus instructif, parce qu'il est involontaire et
contre-intuitif.**

```
MÉCANISME    : les fonctions de paquet de math/rand sont protégées par un
               verrou global. Appelées depuis N goroutines, elles sérialisent
               intégralement l'accès : la section critique devient le chemin
               critique, et l'accélération attendue se transforme en
               ralentissement par contention.
MESURE       : go test -bench Simulate -cpu 1,4,12 -mutexprofile=mutex.out
               go tool pprof -top mutex.out
ATTENDU      : temps croissant avec le nombre de coeurs — la signature
               caractéristique de la contention
```

Un développeur écrit spontanément `rand.Intn()` sans savoir qu'il prend un
verrou. C'est le cas d'école parfait : le code paraît correct, il l'est
fonctionnellement, et il annule le bénéfice du parallélisme.

### Échec C — Mémoïsation non bornée des décisions

```
MÉCANISME    : mettre en cache les décisions dans une map non bornée fait
               croître le tas indéfiniment. Le ramasse-miettes doit alors
               parcourir une structure de plus en plus grande à chaque cycle,
               et son coût finit par dépasser celui du calcul évité — d'autant
               que ce calcul est devenu trivial après le palier 12.
MESURE       : GODEBUG=gctrace=1 go test -bench Simulate ./internal/blackjack
               go test -bench Simulate -memprofile=mem.out
ATTENDU      : cycles de ramasse-miettes plus fréquents et plus longs,
               dégradation croissante avec la durée d'exécution
```

C'est l'exemple de « cache augmentant la pression du GC » cité dans l'énoncé.

### Échec D — Verrouillage trop fin du cache

```
MÉCANISME    : un verrou par entrée de cache multiplie les prises de verrou
               sans réduire la contention réelle, et chaque mutex occupe de la
               mémoire tout en dispersant les données sur plusieurs lignes de
               cache. Un partitionnement par plages, avec bien moins de
               verrous, doit faire mieux.
MESURE       : go test -bench Cache -mutexprofile=mutex.out
ATTENDU      : dégradation par rapport au partitionnement, malgré une
               granularité théoriquement meilleure
```

Troisième exemple cité dans l'énoncé. Un seul de ces quatre échecs sera
documenté en profondeur ; les autres seront mentionnés.

---

## Ordre d'exécution recommandé

```
Phase A  (1 → 3)    gains faciles, validation du protocole de mesure
   ↓
Phase B  (4 → 12)   représentation des données — le palier 5 débloque le reste
   ↓
Échec B             le générateur partagé, AVANT de paralléliser correctement
   ↓
Phase C  (13 → 16)  concurrence, en partant du code déjà optimisé
   ↓
Phase D  (17 → 18)  I/O et persistance
```

**Pourquoi l'échec B avant la phase C.** Tenter d'abord la parallélisation
naïve avec le générateur partagé, constater la régression, puis seulement
appliquer la dérivation de graines : la démonstration est bien plus forte dans
cet ordre, et c'est l'ordre dans lequel un ingénieur rencontre réellement le
problème.

**Pourquoi la phase B avant la phase C.** Paralléliser un code inefficace
multiplie l'inefficacité par le nombre de coeurs. Optimiser le travail d'un
seul coeur d'abord donne une base saine, et rend la mesure d'accélération
interprétable.

---

## Synthèse prévisionnelle

> Ces chiffres sont des **objectifs de travail**, pas des résultats. Ils seront
> remplacés par les mesures réelles dans le rapport d'audit.

| Étape | Débit visé | Allocations par coup | Facteur cumulé |
|---|---|---|---|
| Référence mesurée | 181 000 coups/s | 46 | 1× |
| Après phase A | ~300 000 coups/s | ~30 | ~1,7× |
| Après phase B | ~5 000 000 coups/s | **0** | ~28× |
| Après phase C | ~40 000 000 coups/s | 0 | ~220× |

L'objectif est de **deux ordres de grandeur et demi**, ce qui satisfait
l'exigence de « gains d'ordres de grandeur » du critère 5.

Ce que cela change concrètement : mesurer l'avantage de la maison à 0,01 point
près demande ~1,3 × 10⁸ coups, soit **environ 9 minutes** à la vitesse de
référence et **quelques secondes** en fin de parcours. L'optimisation ne rend
pas le programme « plus rapide » dans l'abstrait — elle rend une mesure
statistique **praticable**.

---

## Rappel final

L'invariant domine tout le reste. À chaque palier :

```bash
go test ./...
```

Un avantage de la maison qui dérive au-delà de 4 erreurs-types signifie que
l'optimisation a cassé la logique de jeu. Elle est annulée, et la régression
est documentée.
