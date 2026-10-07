# Diagnostic et profilage

Identification du chemin chaud à partir de profils réels, et correction de
l'ordre de travail qui en découle.

Ce document répond au **critère 2**. Il contient aussi l'aveu d'une erreur de
méthode : les trois premiers paliers d'optimisation ont été ordonnés **avant**
d'avoir profilé, et le profil montre que l'ordre était partiellement faux. La
leçon vaut mieux qu'un plan reconstruit après coup.

---

## 1. Protocole de génération

```bash
go test -run '^$' -bench Simulate -benchtime 400x \
  -cpuprofile profiles/cpu.out \
  -memprofile profiles/mem.out \
  ./internal/blackjack
```

`-benchtime 400x` fixe un nombre d'itérations plutôt qu'une durée, pour que deux
profils successifs portent sur exactement la même charge.

Une première tentative à `-benchtime 20x` n'avait produit que **280 ms
d'échantillons**, trop peu pour attribuer un coût de façon défendable. Le profil
retenu totalise **7,52 s**, dont 5,48 s dans `Simulate`.

### Lecture

```bash
go tool pprof -top -cum -nodecount=16 profiles/cpu.out       # par fonction
go tool pprof -list 'Shoe..Shuffle' profiles/cpu.out          # ligne par ligne
go tool pprof -sample_index=alloc_objects -top profiles/mem.out
go tool pprof -sample_index=alloc_space  -top profiles/mem.out
```

### Flamegraphs

```bash
go tool pprof -http=:9000 profiles/cpu.out
```

Ouvre l'interface web de pprof, dont l'onglet **Flame Graph** est rendu en
JavaScript et ne requiert aucune dépendance. Les onglets *Graph* et l'export SVG
exigent en revanche Graphviz, absent de la machine de développement :

```bash
winget install Graphviz.Graphviz     # pour l'export SVG du graphe d'appels
```

---

## 2. Profil CPU

Part de `Simulate` (5,48 s), qui est la charge utile mesurée.

| Fonction | Cumulé | Part de `Simulate` |
|---|---|---|
| `PlayRound` | 3,67 s | 67 % |
| **`Shoe.Shuffle`** | **1,76 s** | **32 %** |
| `Hand.Total` | 1,68 s | 31 % |
| `Card.Value` | 1,57 s | 29 % |
| **`runtime.mapaccess1_faststr`** | **1,46 s** | **27 %** |
| `decideBasic` | 1,44 s | 26 % |
| `runtime.mallocgc` | 1,27 s | 23 % |
| `runtime.bgsweep` | 1,16 s | 21 % |

Trois mécanismes ressortent, et aucun n'est propre au métier du blackjack :

1. **Le hachage de chaînes** — `mapaccess1_faststr` à 27 %, auquel s'ajoute
   `aeshashbody` dans les relevés détaillés. C'est le prix des `map[string]`
   utilisées comme tables de correspondance à domaine borné.
2. **L'allocation** — `mallocgc` à 23 %, plus `bgsweep` à 21 % qui est le
   balayage de fond du ramasse-miettes. Le moteur passe donc près de 40 % de son
   temps à allouer et à nettoyer.
3. **Le déplacement de mémoire** — `memmove` et `memclrNoHeapPointers`,
   conséquence du mélange quadratique et de la croissance des slices.

---

## 3. Les quatre lignes qui coûtent la moitié du programme

C'est le résultat le plus exploitable du profil. Quatre lignes de code
totalisent **49 % du temps d'exécution**.

| # | Ligne | Coût cumulé | Part du total |
|---|---|---|---|
| 1 | `card.go:53` — `return cardValues[c.Rank]` | **1,57 s** | **20,9 %** |
| 2 | `strategy.go:166` — `strategyTable[fmt.Sprintf(…)]` | **920 ms** | 12,2 % |
| 3 | `shoe.go:62` — `pool = append(pool, &Card{…})` | 640 ms | 8,5 % |
| 4 | `shoe.go:80` — `pool = append(pool[:i], pool[i+1:]…)` | 540 ms | 7,2 % |

### Annotation de `Shoe.Shuffle`

```
     340ms      1.76s (flat, cum) 23.40% of Total
         .          .     58:	pool := []*Card{}
      10ms       10ms     59:	for d := 0; d < s.numDecks; d++ {
      30ms       30ms     60:		for _, r := range AllRanks {
     100ms      100ms     61:			for _, su := range AllSuits {
      60ms      640ms     62:				pool = append(pool, &Card{Rank: r, Suit: su})
      50ms       50ms     71:	for len(pool) > 0 {
      20ms      220ms     72:		i := s.rng.Intn(len(pool))
      60ms      170ms     73:		shuffled = append(shuffled, pool[i])
      10ms      540ms     80:		pool = append(pool[:i], pool[i+1:]...)
```

La fonction porte **deux défauts distincts et séparément mesurables** :

- **640 ms (36 % de la fonction)** à allouer 208 cartes une par une sur le tas,
  ligne 62 ;
- **540 ms (31 %)** dans le décalage provoqué par chaque retrait, ligne 80 —
  c'est le coût quadratique, confirmé par le comptage instrumenté à
  10 756 déplacements par rebattage contre 208 pour Fisher-Yates.

S'y ajoutent 170 ms de croissance du slice résultat, et 220 ms de générateur
pseudo-aléatoire.

### Annotation de `Hand.Total`

```
     190ms      1.68s (flat, cum) 22.34% of Total
     130ms      130ms     39:	for _, c := range h.Cards {
         .      1.43s     40:		total += c.Value()
      10ms       70ms     41:		if c.IsAce() {
```

Le coût de `Total` **n'est pas dans sa logique** : 1,43 s de ses 1,68 s partent
dans `Card.Value`, c'est-à-dire dans une consultation de map. Optimiser le calcul
incrémental du total sans toucher à la représentation de la carte ne gagnerait
donc presque rien — ce qui invalide par avance la priorité que j'avais donnée au
palier « total incrémental ».

### Annotation de `decideBasic`

```
      50ms      1.44s (flat, cum) 19.15% of Total
      30ms      920ms    166:	d, ok := strategyTable[fmt.Sprintf("%s-%d-%s", kind, total, upKey)]
      10ms      120ms    147:	if h.IsPair() && handCount < r.MaxSplitHands {
```

Une seule ligne concentre **920 ms sur 1,44 s**, soit 64 % de la fonction. Elle
cumule un formatage, une allocation et un hachage de chaîne.

---

## 4. Profil d'allocations

Sur 2 774 Mo alloués et 63,6 millions d'objets.

| Fonction | Objets | Octets |
|---|---|---|
| **`Shoe.Shuffle`** | **48,0 %** | **73,3 %** |
| `PlayRound` | 19,0 % | 12,5 % |
| `decideBasic` | 17,9 % | 6,2 % |
| `fmt.Sprintf` | 8,4 % | 2,9 % |
| `Hand.Add` | 6,6 % | 4,8 % |

`Shoe.Shuffle` alloue à elle seule **près des trois quarts des octets** du
programme. C'est sans comparaison le premier poste mémoire.

---

## 5. Identification formelle du chemin chaud

> Le moteur est limité par **trois mécanismes du runtime**, et non par la logique
> du blackjack : le hachage de chaînes utilisées comme clés de table (27 % du
> CPU), l'allocation sur le tas et son ramassage (≈ 40 % du CPU), et le
> déplacement de mémoire provoqué par un mélange quadratique.
>
> Le chemin chaud se concentre dans **quatre lignes** représentant 49 % du temps,
> toutes situées dans trois fonctions : `Shoe.Shuffle`, `Card.Value` — appelée
> depuis `Hand.Total` — et `decideBasic`.
>
> Aucune de ces quatre lignes n'exprime une règle du jeu. Toutes sont des choix
> de représentation : une carte décrite par deux chaînes, des tables de
> correspondance implémentées en `map[string]`, et un sabot reconstruit à chaque
> rebattage.

### Contention et blocage

`runtime.lock2` apparaît marginalement au profil, cohérent avec un moteur
strictement mono-thread. Le parallélisme effectif mesuré à **1,32 cœur** pour un
programme séquentiel s'explique entièrement par le ramasse-miettes travaillant
sur les autres cœurs — ce qui est en soi un argument : la pression d'allocation
n'occupe pas seulement le cœur de calcul, elle en mobilise d'autres.

Le profil de contention de verrous sera pertinent à partir de la phase de
concurrence, pas avant.

---

## 6. Erreur de méthode : trois paliers ordonnés sans profil

Les paliers 1 à 3 ont été choisis **avant** ce profilage, par raisonnement a
priori. Le profil montre que l'ordre était partiellement faux.

| Palier | Ce qu'il visait | Ce que le profil en dit |
|---|---|---|
| 1 — journal narratif | `fmt.Sprintf` dans `PlayRound` | **Juste.** Plus gros gain disponible, −47 % et ×2,35 de bout en bout. Correction de conception, pas micro-optimisation. |
| 2 — tables de gains des paris annexes | `map[string]float64` | **Faux.** Les paris annexes n'apparaissent pas dans le top 16 du profil. Gain réel : 0 % au niveau système. |
| 3 — dévirtualisation de la stratégie | indirection d'interface | **Bonne zone, mauvais mécanisme.** `decideBasic` pèse 19 %, mais 64 % de ce coût est dans la ligne de `map` + `Sprintf`, pas dans l'appel dynamique. |

Et le plus net : **`Shoe.Shuffle` était placé aux paliers 7 et 8 sur 18**, alors
qu'elle représente 32 % du CPU et 73 % des octets alloués.

### Ce que cela coûte, et pourquoi le dire

Deux paliers sur trois ont été dépensés sur des cibles secondaires. Ce n'est pas
grave en soi — les mesures sont justes, la correction est préservée — mais c'est
une démonstration directe de la raison d'être du critère 2 : **sans profil, le
raisonnement a priori se trompe sur l'ordre, même quand il identifie les bons
problèmes.**

J'avais bien listé le mélange et les maps dans la feuille de route. Ce que je ne
pouvais pas deviner, c'est leur poids relatif.

---

## 7. Ordre corrigé, dicté par le profil

| Rang | Cible | Ce qu'elle attaque | Poids mesuré |
|---|---|---|---|
| **1** | Mélange de Fisher-Yates en place | ligne 80, le décalage quadratique | 540 ms, 7,2 % |
| **2** | Carte sur 1 octet, stockée par valeur | ligne 53 (map) **et** ligne 62 (allocation) | 1,57 s + 640 ms, 29 % |
| **3** | Sabot en tableau fixe et curseur | le reste des allocations de `Shuffle` | 73 % des octets |
| **4** | Table de stratégie plate indexée | ligne 166 | 920 ms, 12 % |
| 5 | Mains en tableaux fixes, `Hand.Add` | 6,6 % des objets | — |
| 6 | Total incrémental | **déclassé** : son coût est dans `Card.Value`, déjà traité au rang 2 | — |

### Deux regroupements que le profil justifie

**Les rangs 1 et 3 touchent la même fonction** mais des lignes différentes — le
décalage quadratique d'un côté, l'allocation des cartes de l'autre. Les mesurer
séparément est possible et souhaitable, puisque le profil les distingue.

**Le rang 2 ne peut pas être scindé.** On ne peut pas indexer un tableau par le
rang d'une carte sans que ce rang soit numérique : la compaction de `Card` et le
remplacement de la map sont un seul et même changement. Mon ancien palier
intermédiaire « `map` → `switch` » devient inutile — il atténuerait le problème
au lieu de le supprimer.

### Un déclassement assumé

Le palier « total incrémental » passe de la priorité 9 à la dernière place. Le
profil est formel : 1,43 s des 1,68 s de `Hand.Total` sont dans `Card.Value`.
Une fois la carte compactée, il ne restera presque rien à gagner. L'optimiser
avant aurait été du travail perdu.

---

## 8. Reproduire ce diagnostic

```bash
make profile        # génère profiles/cpu.out et profiles/mem.out
make profile-top    # les tableaux de la section 2
make profile-lines  # les annotations de la section 3
```

Les profils eux-mêmes ne sont pas versionnés : ils sont régénérables, et dépendent
du matériel. Seules leurs conclusions le sont, dans ce document.
