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
```

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

Relevée sur la machine de développement, Go 1.26.4, windows/amd64, 12 coeurs
logiques dont **un seul utilisé**.

De bout en bout, sur le binaire, **15 exécutions de 500 000 coups**, machine
sur secteur et au repos :

| Métrique | Valeur |
|---|---|
| Débit médian | **342 306 coups/s** |
| Temps par coup | **2 921 ns** |
| Écart-type | 15 363 coups/s — **4,52 %** |
| Étendue min–max | 297 537 – 355 637 coups/s (17 % de la médiane) |

> **Avertissement de protocole, appris à nos dépens.** Une première mesure
> isolée avait donné 181 112 coups/s, soit **1,9 fois moins**. Elle avait été
> prise juste après l'exécution de la suite de tests et des benchmarks, sur une
> machine encore chargée et thermiquement sollicitée — un AMD Ryzen 5 5600H est
> un processeur mobile dont la fréquence dépend fortement de son état.
>
> Une mesure unique sur ce matériel ne vaut rien. Toute valeur rapportée ici
> est une **médiane sur 15 exécutions**, accompagnée de son écart-type, machine
> au repos et sur secteur.

Par benchmark Go, avec `-benchmem` :

| Benchmark | Temps | Mémoire | Allocations |
|---|---|---|---|
| `PlayRound` — un coup complet | 5 200 ns/op | 1 656 o/op | **46 allocs/op** |
| `PlayRound` avec paris annexes | 4 900 ns/op | 2 081 o/op | **57 allocs/op** |
| `Shuffle` — un rebattage | 19 300 ns/op | 15 488 o/op | **220 allocs/op** |
| `Decide` — une décision | 264 ns/op | 48 o/op | 3 allocs/op |
| `HandTotal` | 36,6 ns/op | 0 | 0 |
| `Simulate` — 10 000 coups | 41 ms/op | **16,5 Mo/op** | **469 801 allocs/op** |

Les 16,5 Mo alloués pour 10 000 coups représentent **1,65 Go par million de
coups** : la pression sur le ramasse-miettes est le premier suspect du profil.

Tailles des structures, relevées par `unsafe.Sizeof` (voir `TestStructSizes`) :

| Structure | Taille | Champs utiles | Remplissage |
|---|---|---|---|
| `Card` | 32 o | 32 o | 0 o |
| `Hand` | 56 o | 37 o | **19 o (34 %)** |
| `RoundResult` | 104 o | 76 o | **28 o (27 %)** |

Une carte tiendrait dans **un seul octet** — 4 bits de rang, 2 bits d'enseigne.
Le facteur sur la représentation est donc de **32**, et une ligne de cache de
64 octets contient 2 cartes au lieu de 64.

Ces chiffres n'ont de valeur qu'accompagnés de la spécification complète du
banc d'essai et d'un protocole statistique — voir [docs/](docs/).

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

## 12. Feuille de route d'optimisation

Chaque palier fait l'objet d'une branche, d'une mesure isolée par `benchstat`
et d'une entrée au journal d'optimisation.

**Mémoire et localité de cache**

1. Journal narratif rendu explicite, retiré du chemin de simulation
2. `map[string]int` → tableau indexé pour la valeur des cartes
3. `[]*Card` → `[]Card` : suppression de l'allocation par carte
4. `Card` compactée sur 1 octet (4 bits de rang, 2 bits d'enseigne)
5. Sabot en tableau fixe distribué par curseur, zéro allocation
6. Mélange de Fisher-Yates en place
7. Total de la main maintenu en incrémental
8. Réordonnancement des champs de `Hand` et `RoundResult`
9. Mains en tableaux fixes, suppression du dernier `append`

**Concurrence et scalabilité**

10. Stratégie dévirtualisée, table plate indexée arithmétiquement
11. Worker pool borné aux coeurs physiques, générateur par worker
12. Agrégation atomique ou fusion locale sans contention
13. Arrêt précoce sur critère d'intervalle de confiance

**I/O réseau et persistance**

14. API binaire Protobuf/gRPC comparée à la sérialisation JSON
15. Persistance indexée des résultats par jeu de règles, `EXPLAIN ANALYZE`
16. Cache LRU des configurations déjà simulées

L'invariant est absolu : **l'avantage de la maison mesuré ne doit pas bouger**.
`go test ./...` avant chaque fusion, sans exception.

Chaque palier est détaillé dans
[docs/01-perspectives-optimisation.md](docs/01-perspectives-optimisation.md) :
hypothèse d'impact matériel, commande de vérification, condition de réfutation,
et axe de la grille visé. Les quatre échecs planifiés du critère 4 y sont
également documentés, avec leur mécanisme et la mesure qui les révèle.
