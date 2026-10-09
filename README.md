# american-blackjack

Moteur de simulation de table de blackjack américain en Go, écrit pour servir
de support à un audit d'optimisation de performances backend.

Le but n'est pas de jouer une partie, mais d'en jouer des dizaines de millions
pour mesurer l'avantage de la maison. La métrique centrale du projet est donc
le **débit du moteur**, en coups simulés par seconde.

> **Important** — l'état actuel du dépôt est la **version de référence
> (baseline)** : volontairement naïve dans ses choix de représentation, mais
> rigoureusement correcte dans sa logique de jeu. C'est ce double caractère qui
> permet de prouver qu'une optimisation accélère le moteur *sans* changer son
> résultat.

---

## 1. Pourquoi la performance compte ici

Ce n'est pas un exercice gratuit. L'écart-type du résultat par coup au
blackjack vaut environ **1,14 unité de mise**, soit plus de trois fois
l'avantage de la maison que l'on cherche à mesurer (~0,35 %).

Conséquence directe : l'erreur-type de la mesure décroît en `1/√n`. Pour
résoudre l'avantage de la maison à 0,01 point près, il faut de l'ordre de
**1,3 × 10⁸ coups par exécution**.

| Coups simulés | Erreur-type sur l'avantage | Durée à la vitesse baseline |
|---|---|---|
| 10⁶ | ± 0,114 point | ~3 s |
| 10⁷ | ± 0,036 point | ~29 s |
| 10⁸ | ± 0,011 point | ~5 min |
| 10⁹ | ± 0,004 point | ~49 min |

Le débit du moteur n'est donc pas une coquetterie : il conditionne la
**précision statistique atteignable**. C'est la justification physique de tout
le travail d'optimisation mené dans ce dépôt.

---

## 2. Règles implémentées

Blackjack **américain** : le croupier reçoit une carte visible et une carte
cachée dès la distribution, et contrôle sa carte cachée (*peek*) quand sa carte
visible est un As ou une bûche.

### Table

- **4 jeux** (208 cartes), mélangés par Fisher-Yates
- **Carte de coupe** à 75 % de pénétration ; rebattage entre deux coups, jamais
  au milieu d'un coup
- Croupier tire jusqu'à 16, reste à 17 — bascule **S17 / H17** disponible
- Blackjack payé **3:2** (paramétrable, 6:5 inclus)
- **Assurance** proposée sur un As visible
- **Abandon tardif**, après le contrôle de la carte cachée

### Joueur

- Tirer, rester, **doubler**, **séparer**, abandonner
- Double sur n'importe quelles deux cartes, ou restreint aux totaux 9–11
- **Double après séparation (DAS)**
- Séparation jusqu'à **4 mains**
- As séparés limités à une carte ; reséparation des As paramétrable
- Un 21 obtenu après séparation n'est **pas** un blackjack

Le joueur applique la **stratégie de base** pour 4 à 8 jeux en S17, avec
dégradation automatique quand la règle de la table interdit la décision
optimale (double impossible, séparations épuisées).

---

## 3. Paris annexes

Quatre paris annexes, activables indépendamment. Ils imposent de suivre les
**enseignes** des cartes, et non seulement leurs rangs.

### Perfect Pairs

Les deux premières cartes du joueur forment une paire.

| Combinaison | Gain |
|---|---|
| Paire parfaite (même rang, même enseigne) | 25:1 |
| Paire colorée (même rang, même couleur) | 12:1 |
| Paire mixte (même rang, couleurs différentes) | 6:1 |

### 21+3

Les deux cartes du joueur et la carte visible du croupier forment une main de
poker à trois cartes.

| Combinaison | Gain |
|---|---|
| Brelan de même enseigne | 100:1 |
| Quinte flush | 40:1 |
| Brelan | 30:1 |
| Quinte | 10:1 |
| Couleur | 5:1 |

L'As compte comme 14 (Dame-Roi-As) et comme 1 (As-2-3).

### Lucky Ladies

Les deux premières cartes du joueur totalisent 20.

| Combinaison | Gain |
|---|---|
| Deux Dames de coeur + blackjack du croupier | 1000:1 |
| Deux Dames de coeur | 200:1 |
| 20 formé de deux cartes identiques | 25:1 |
| 20 de même enseigne | 10:1 |
| 20 quelconque | 4:1 |

### Buster Blackjack

Le croupier saute, d'autant mieux payé que sa main compte de cartes.

| Cartes du croupier | Gain |
|---|---|
| 3–4 | 2:1 |
| 5 | 4:1 |
| 6 | 18:1 |
| 7 | 50:1 |
| 8 ou plus | 250:1 |

---

## 4. Lancement

Prérequis : **Go 1.26** ou supérieur.

```bash
# Simulation simple
go run ./cmd/simulate -rounds 2000000 -seed 42

# Avec tous les paris annexes
go run ./cmd/simulate -rounds 2000000 -perfect-pairs 1 -21plus3 1 -lucky-ladies 1 -buster 1

# Variante de règles défavorable
go run ./cmd/simulate -rounds 2000000 -h17 -decks 8 -surrender=false

# Débit seul, pour les mesures automatisées
go run ./cmd/simulate -rounds 2000000 -quiet

# Parallèle : un worker par coeur logique
go run ./cmd/simulate -rounds 20000000 -workers 0

# Nombre de workers imposé, pour tracer la courbe d'accélération
go run ./cmd/simulate -rounds 2000000 -workers 6
```

`-workers 1` est le **mode de référence** : strictement séquentiel, c'est lui
qui sert à toutes les comparaisons de paliers. `-workers 0` prend
`runtime.NumCPU()`.

`-quiet` n'émet que le nombre de coups par seconde, ce qui le rend directement
consommable par un script de mesure.

---

## 5. Interface web

Une table jouable, servie par un serveur Go qui réutilise le moteur.

```bash
go run ./cmd/server
```

Puis **http://localhost:8090**.

```bash
# Partie reproductible et table plus défavorable
go run ./cmd/server -seed 42 -decks 8 -h17 -bankroll 500

# Autre port si 8090 est déjà pris
go run ./cmd/server -addr :8091
```

> Le port d'écoute est **8090** et non 8080, parce que ce dernier est très
> souvent occupé — notamment par le listener HTTP d'Oracle XE (`TNSLSNR`), qui
> répond un `401` et donne l'impression trompeuse que c'est notre serveur qui
> réclame un mot de passe. En cas de conflit, le serveur refuse de démarrer
> avec un message explicite au lieu d'annoncer une URL injoignable.

Trois onglets :

- **Table** — jeu complet avec les quatre paris annexes, séparations, doubles,
  assurance et abandon. Raccourcis clavier `T` tirer, `R` rester, `D` doubler,
  `S` séparer, `A` abandonner, `Entrée` distribuer. Une case à cocher affiche
  le conseil de la stratégie de base sur le bouton recommandé.
- **Stratégie** — la table de stratégie de base en grille colorée, **lue depuis
  le code** et non recopiée, donc garantie identique à ce que la simulation
  applique.
- **Statistiques** — un **tableau de bord temps réel**. La simulation est
  déroulée par lots et diffusée en Server-Sent Events&nbsp;: quatre courbes se
  tracent en direct — débit, taux d'allocation, part CPU du ramasse-miettes, et
  avantage de la maison avec sa bande de confiance à 95 % — accompagnées d'une
  barre de progression et d'une estimation du temps restant. S'y ajoutent la
  courbe de convergence en échelle logarithmique, la comparaison chiffrée des
  variantes de règles avec barres d'erreur, et le récit narré d'un coup.

> **Le tableau de bord n'est pas un instrument de mesure.** Le déroulement par
> lots nécessaire à l'affichage en direct relève les compteurs du runtime entre
> chaque lot, ce qui perturbe le régime permanent — on mesure environ
> 255 000 coups/s en streaming contre 342 306 en un seul bloc. L'interface
> affiche cet avertissement en permanence. Le chiffre qui fait foi est celui de
> `cmd/simulate`, et le travail de métrologie passe par le harnais, jamais par
> une requête HTTP.

### Deux garde-fous d'architecture

**Le serveur ne touche pas au chemin mesuré.** L'interface s'appuie sur un type
`Table` distinct de `PlayRound` : la boucle de simulation reste non
instrumentée, sans champ ajouté pour l'affichage et sans indirection, puisque
c'est elle qui est mesurée. Les structures `Card`, `Hand` et `Shoe` ne portent
aucun champ de présentation — l'état propre au jeu interactif vit dans des
types séparés (`view.go`).

**Aucune règle n'est implémentée côté navigateur.** La page affiche un état et
transmet une action ; le serveur arbitre tout. Ce qu'on joue dans l'interface
est donc exactement ce que le moteur simule, et `TestTableCrossValidation` le
prouve en mesurant l'avantage de la maison par les deux chemins.

---

## 6. Harnais de mesure

Toute la campagne de mesure, en **une seule commande** :

```bash
bash scripts/run_benchmarks.sh
```

Elle produit `results/<horodatage>/` contenant le relevé du banc d'essai, la
validation de l'oracle, les benchmarks comparés au tag `v0-baseline`, la mesure
de bout en bout par `hyperfine`, les compteurs d'opérations, les profils CPU et
allocations avec leurs annotations ligne par ligne, et un résumé citable
mentionnant la révision Git mesurée.

Étapes isolables : `hardware`, `test`, `bench`, `hyperfine`, `profile`.

Avec `make`, si disponible : `make measure`, `make check`, `make oracle`,
`make profile-lines`, `make flame`.

Prérequis :

```bash
go install golang.org/x/perf/cmd/benchstat@latest
winget install sharkdp.hyperfine
```

### Deux refus délibérés

Le harnais **refuse de mesurer sur batterie** et interrompt la campagne. Une
mesure sur batterie n'est pas imprécise, elle est fausse : le même binaire a
donné 181 112 et 342 306 coups/s selon l'état de la machine.

Le harnais **s'arrête si l'oracle échoue**, avant de lancer le moindre
benchmark. Chiffrer les performances d'un moteur dont la logique est cassée n'a
aucun sens.

---

## 7. Métriques

Le moteur rapporte son propre comportement, sans jamais être instrumenté dans
la boucle mesurée.

```bash
go run ./cmd/simulate -rounds 500000
```

Produit le débit (coups, mains et cartes par seconde), le temps par coup en
horloge **et** en CPU, le parallélisme effectif, les octets et allocations par
coup, le taux d'allocation, les cycles et pauses du ramasse-miettes avec sa part
de CPU, et la latence de l'ordonnanceur.

```bash
go run ./cmd/simulate -rounds 500000 -json      # pour un script de mesure
go run ./cmd/simulate -rounds 500000 -quiet     # débit seul, pour hyperfine
```

### Comptage des opérations

```bash
go run -tags instrument ./cmd/simulate -rounds 200000
```

Ajoute le nombre d'opérations élémentaires : décisions, appels à `Total()`,
évaluations de carte, consultations de map, déplacements de mélange.

**Ces compteurs sont absents du binaire par défaut.** `Instrumented` est une
constante fausse, donc chaque bloc de comptage est éliminé à la compilation.
Preuve par le code machine :

```bash
go tool objdump -s 'blackjack\.\(\*Hand\)\.Total' def.exe | wc -l   # 0
go tool objdump -s 'blackjack\.\(\*Hand\)\.Total' ins.exe | wc -l   # 77
```

Dans le binaire par défaut, `Hand.Total` **n'existe pas comme fonction** : elle
est entièrement inlinée. Dans le binaire instrumenté elle redevient une fonction
autonome, car l'appel atomique empêche l'inlining. Le coût de l'instrumentation
n'est donc pas une incrémentation, c'est **la perte de l'inlining** — raison
pour laquelle elle doit être compilée dehors et non simplement tolérée.

Protocole complet et banc d'essai :
[docs/02-protocole-de-mesure.md](docs/02-protocole-de-mesure.md).

---

## 8. Oracle de non-régression

C'est la pièce maîtresse du dispositif. L'avantage de la maison au blackjack
est une grandeur **publiée** : le moteur doit la reproduire, ce qui permet de
détecter toute optimisation qui casserait subtilement la logique de jeu.

```bash
go test ./... -v
```

Valeurs de contrôle mesurées sur la baseline, 2 × 10⁶ coups, graine 42 :

| Grandeur | Mesuré | Attendu | Écart |
|---|---|---|---|
| Avantage de la maison | +0,2641 % | ~0,35 % | 1,06 erreur-type |
| Écart-type par coup | 1,1414 | ~1,14 | conforme |
| Blackjacks joueur | 4,729 % | ~4,75 % | conforme |
| Dépassement du croupier | 28,223 % | ~28,3 % | conforme |

Quatre validations indépendantes qui convergent. S'y ajoutent deux contrôles
externes : `TestTableCrossValidation` fait concorder la table interactive et la
boucle simulée à **0,0040 point**, et la comparaison des variantes retrouve la
pénalité du blackjack 6:5 à **+1,374 point** contre +1,39 publié. `TestHouseEdgeOracle` borne
l'écart accepté à **4 erreurs-types** de la valeur publiée, calculées à partir
de la variance réellement observée et non d'une marge arbitraire.

`TestDeterminisme` vérifie par ailleurs qu'à graine égale, deux simulations
produisent des statistiques rigoureusement identiques.

### Deux pièges de mesure déjà identifiés

**Le dépassement du croupier ne se mesure pas en cours de partie.** Le croupier
ne complète pas sa main quand toutes les mains du joueur ont sauté — or ce sont
justement les coups où il montrait une carte forte, qui saute peu. Le taux
conditionnel est donc biaisé vers le haut (31 % observé) et doit être validé
séparément, sur des mains systématiquement jouées (28,185 %).

**Le pari Buster modifie le jeu principal.** Quand il est actif, le croupier
complète sa main dans 95 % des coups au lieu de 73 %, ce qui consomme plus de
cartes et décale l'avantage du jeu principal. L'oracle doit donc se mesurer
**sans paris annexes**.

---

## 9. Mesure de référence

Conditions de la section 6 — machine sur secteur, au repos.

### Bout en bout

| Mode | Débit | Temps par coup |
|---|---|---|
| Version de référence (tag `v0-baseline`) | 342 306 coups/s | 2 921 ns |
| Séquentiel optimisé | **6 026 483 coups/s** | 166 ns |
| **12 workers** | **34 485 553 coups/s** | **31 ns** |

Soit **×17,6 en séquentiel** et **×100,7 en parallèle** depuis la baseline.

### Courbe d'accélération

Mesurée sur 200 000 coups par itération, `-benchtime 3s -count 6`.

| Workers | Débit | Speedup |
|---|---|---|
| 1 | 5 829 390/s | 1,00× |
| 2 | 8 888 809/s | 1,52× |
| 4 | 15 044 784/s | 2,58× |
| **6** — cœurs physiques | 20 651 043/s | **3,54×** |
| 8 | 24 743 210/s | 4,24× |
| **12** — cœurs logiques | 27 709 084/s | **4,75×** |

L'accélération est **sous-linéaire**, et pour deux raisons mesurables. Jusqu'à
6 workers, les cœurs physiques se partagent la bande passante mémoire — or la
machine n'a **qu'une barrette**, donc un seul canal. Au-delà de 6, les workers
se partagent les unités de calcul d'un même cœur physique : l'hyperthreading
masque des latences mais ne double pas le silicium, et n'apporte que ×1,34
supplémentaire.

### Benchmarks

| Benchmark | Baseline | Actuel | Gain |
|---|---|---|---|
| `PlayRound` | 3 731 ns / 46 allocs | **164,5 ns / 0 alloc** | −95,6 % |
| `Decide` | 217,8 ns / 3 allocs | **9,1 ns / 0 alloc** | −95,8 % |
| `Shuffle` | 21,2 µs / 220 allocs | 1,75 µs / 1 alloc | −91,8 % |
| `HandTotal` | 31,3 ns | 3,2 ns | −89,7 % |
| `Simulate` 10⁴ coups | 34,0 ms / 469 800 allocs | 1,75 ms / **344 allocs** | −94,9 % |

### Comportement mémoire

| Grandeur | Baseline | Actuel |
|---|---|---|
| Allocations par coup | 46 | **0,03** |
| Cycles de GC sur 2 M coups | 244 | **4** |
| Part CPU du ramasse-miettes | 15,25 % | **0,00 %** |
| Parallélisme effectif, mode séquentiel | 1,32 cœur | **1,01 cœur** |

Le parallélisme effectif à 1,01 en mode séquentiel signifie que le
ramasse-miettes ne mobilise plus aucun autre cœur.

## 10. Choix volontairement naïfs de la baseline

Chacun est documenté dans le code à l'endroit où il est fait, avec son coût
physique. Ce sont les cibles du travail d'optimisation.

**Règle appliquée** : chaque défaut est du code qu'un développeur débutant
écrirait réellement. Aucun ralentissement artificiel — pas d'attente, pas de
travail inventé. C'est la condition pour que les gains mesurés soient
justifiables physiquement, et non fabriqués.

| Emplacement | Choix naïf | Pourquoi c'est plausible | Coût |
|---|---|---|---|
| `card.go` | rang et enseigne en `string` | on écrit ce qu'on lit | `Card` fait 32 o au lieu de 1 o |
| `card.go` | `map[string]int` pour la valeur d'une carte | plus lisible qu'un `switch` | un hachage par carte et par appel à `Total()` |
| `hand.go`, `shoe.go` | `[]*Card` au lieu de `[]Card` | réflexe Java/C# | une allocation par carte, aucune localité |
| `hand.go` | total recalculé à chaque appel | le plus simple à écrire | parcours complet, plusieurs fois par décision |
| `hand.go`, `round.go` | champs de structs dans un ordre quelconque | on n'y pense pas | 19 o et 28 o de remplissage |
| `shoe.go` | mélange par tirage-et-retrait | l'algorithme intuitif | **10 756 déplacements par rebattage** contre 208 pour Fisher-Yates |
| `shoe.go` | sabot reconstruit par `append` | on repart de zéro | 15,5 Ko et 220 allocations par rebattage |
| `strategy.go` | clés textuelles via `fmt.Sprintf` dans une `map` | une table se fait avec une map | 3 allocations par décision |
| `strategy.go` | stratégie derrière une interface | réflexe orienté objet | appel dynamique, pas d'inlining |
| `round.go` | journal narratif par `fmt.Sprintf` | on veut afficher l'historique | ~10 allocations par coup, jetées aussitôt |
| `sidebets.go` | tables de gains en `map[string]float64` | idem | hachage de chaîne sur le hot path |
| `sim.go` | boucle strictement séquentielle | on n'y pense pas d'emblée | 1 coeur sur 12 |

### Un cas à part : le journal narratif

Le journal construit par `PlayRound` n'est **pas** du travail inutile : il
alimente le panneau de narration de l'interface, via `/api/sample-round`.

Le défaut n'est pas de le produire, c'est de le produire **sans que l'appelant
l'ait demandé** — donc aussi dans la boucle de simulation, qui ne le lit
jamais. C'est la forme la plus courante de gaspillage en production : un code
partagé entre deux usages paie le coût du plus exigeant des deux. L'optimisation
consistera à le rendre explicite, pas à le supprimer.

## 11. Structure

```
.
├── cmd/simulate/          # commande de simulation et rapport
├── cmd/server/            # serveur web et interface de table
│   └── web/               # page, styles et script de l'interface
├── internal/blackjack/
│   ├── card.go            # représentation d'une carte
│   ├── hand.go            # main, total, blackjack, paire
│   ├── shoe.go            # sabot, mélange, carte de coupe
│   ├── rules.go           # variantes de règles paramétrables
│   ├── strategy.go        # stratégie de base et dégradation
│   ├── sidebets.go        # évaluation des quatre paris annexes
│   ├── round.go           # déroulement d'un coup complet
│   ├── sim.go             # boucle de simulation et statistiques
│   ├── table.go           # table jouable coup par coup (hors chemin mesuré)
│   ├── view.go            # sérialisation de l'état vers l'interface
│   ├── analysis.go        # données des figures du rapport
│   ├── runner.go          # déroulement par lots (hors chemin mesuré)
│   ├── counters_off.go    # compteurs absents du build par défaut
│   ├── counters_on.go     # compteurs du build -tags instrument
│   ├── blackjack_test.go  # oracle et tests de correction
│   └── table_test.go      # validation croisée Table / PlayRound
├── internal/metrics/       # métriques runtime, lues autour de la boucle
├── scripts/
│   ├── run_benchmarks.sh  # LE harnais, une seule commande
│   └── hardware.ps1       # relevé du banc d'essai
├── Makefile               # table des matières des commandes
├── bench/                 # relevés de mesure versionnés
├── docs/
│   ├── 00-grille-et-plan.md             # suivi de la couverture des critères
│   ├── 01-perspectives-optimisation.md  # les 18 paliers, hypothèse par hypothèse
│   └── 02-protocole-de-mesure.md        # banc d'essai et protocole statistique
└── constitution.md        # gouvernance technique des assistants IA
```

---

## 12. Paliers d'optimisation réalisés

Chaque palier a sa branche, sa mesure `benchstat` dans [bench/](bench/) et son
commit dédié. L'ordre a été **dicté par le profil**, pas par un plan a priori —
voir [docs/03-profiling.md](docs/03-profiling.md).

| # | Palier | Gain principal |
|---|---|---|
| 1 | Journal narratif rendu explicite | `PlayRound` −47 % |
| 2 | Combinaisons des paris annexes en entiers | local −11,6 %, **neutre au global** |
| 3 | Stratégie dévirtualisée | `Decide` −10,6 % |
| 4 | Mélange de Fisher-Yates en place | `Shuffle` −42 %, O(n²) → O(n) |
| 5 | **Carte sur un octet, stockée par valeur** | geomean −83 % |
| 6 | Réordonnancement des champs | `Hand` 56 → 40 o |
| 7 | **Table de stratégie plate indexée** | `Decide` −95 % |
| 8 | **Mains en tableaux fixes sur la pile** | **0 allocs/op** |
| 9 | **Worker pool borné** | **×4,75 sur 12 cœurs** |

### Échec constructif

La mémoïsation de `Hand.Total` a été tentée, mesurée, et **annulée** : −72,6 %
de débit. Branche `echec/memoisation-total` conservée, analyse dans
[docs/04-echec-constructif.md](docs/04-echec-constructif.md).

### Ce qui reste

L'axe **I/O réseau et persistance** du critère 3 n'est pas traité : streaming
binaire à la place du JSON WebSocket, et persistance indexée des résultats.

---

## 13. Invariant

L'avantage de la maison mesuré ne doit jamais dévier de plus de 4 erreurs-types
de la valeur publiée. `go test ./...` avant chaque fusion, sans exception.

Sur les neuf paliers, il est resté **à l'identique au chiffre près** à chaque
fois, sauf aux deux endroits où l'ordre de consommation du générateur change
légitimement — Fisher-Yates et la parallélisation par graines dérivées. Dans ces
deux cas la validation croisée `Table` / `PlayRound` a confirmé que seules les
cartes avaient changé, pas les règles.
