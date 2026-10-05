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
| 10⁶ | ± 0,114 point | ~2 s |
| 10⁷ | ± 0,036 point | ~20 s |
| 10⁸ | ± 0,011 point | ~3 min 20 s |
| 10⁹ | ± 0,004 point | ~33 min |

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

Puis http://localhost:8080.

```bash
# Partie reproductible et table plus défavorable
go run ./cmd/server -seed 42 -decks 8 -h17 -bankroll 500
```

Trois onglets :

- **Table** — jeu complet avec les quatre paris annexes, séparations, doubles,
  assurance et abandon. Raccourcis clavier `T` tirer, `R` rester, `D` doubler,
  `S` séparer, `A` abandonner, `Entrée` distribuer. Une case à cocher affiche
  le conseil de la stratégie de base sur le bouton recommandé.
- **Stratégie** — la table de stratégie de base en grille colorée, **lue depuis
  le code** et non recopiée, donc garantie identique à ce que la simulation
  applique.
- **Statistiques** — lancement de simulations, courbe de convergence de
  l'avantage de la maison en échelle logarithmique avec sa bande de confiance à
  95 %, et comparaison chiffrée des variantes de règles avec barres d'erreur.

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

## 6. Oracle de non-régression

C'est la pièce maîtresse du dispositif. L'avantage de la maison au blackjack
est une grandeur **publiée** : le moteur doit la reproduire, ce qui permet de
détecter toute optimisation qui casserait subtilement la logique de jeu.

```bash
go test ./... -v
```

Valeurs de contrôle mesurées sur la baseline, 2 × 10⁶ coups, graine 42 :

| Grandeur | Mesuré | Attendu | Écart |
|---|---|---|---|
| Avantage de la maison | +0,3164 % | ~0,35 % | 0,42 erreur-type |
| Écart-type par coup | 1,1411 | ~1,14 | conforme |
| Blackjacks joueur | 4,732 % | ~4,75 % | conforme |
| Dépassement du croupier | 28,185 % | ~28,3 % | conforme |

Quatre validations indépendantes qui convergent. `TestHouseEdgeOracle` borne
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

## 7. Mesure de référence

Relevée sur la machine de développement, Go 1.26.4, windows/amd64 :

| Métrique | Valeur |
|---|---|
| Débit | **504 540 coups/s** |
| Temps par coup | **1 982 ns** |

Ce chiffre n'a de valeur qu'accompagné de la spécification complète du banc
d'essai et d'un protocole statistique — voir [docs/](docs/).

---

## 8. Choix volontairement naïfs de la baseline

Chacun est documenté dans le code à l'endroit où il est fait, avec son coût
physique. Ce sont les cibles du travail d'optimisation à venir.

| Emplacement | Choix naïf | Coût |
|---|---|---|
| `card.go` | rang et enseigne en `string` | `Card` fait 32 o au lieu de 1 o ; 2 cartes par ligne de cache au lieu de 64 |
| `hand.go` | cartes en slice, total recalculé | allocation par tirage, parcours complet à chaque appel |
| `shoe.go` | sabot reconstruit par `append`, distribution par re-slicing | ~6,6 Ko alloués par rebattage, des milliers de fois |
| `strategy.go` | clés textuelles via `fmt.Sprintf` dans une `map` | une allocation et un hachage par décision |
| `sidebets.go` | tables de gains en `map[string]float64` | hachage de chaîne sur le hot path |
| `sim.go` | boucle strictement séquentielle | un seul coeur utilisé sur 12 |

---

## 9. Structure

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
│   ├── blackjack_test.go  # oracle et tests de correction
│   └── table_test.go      # validation croisée Table / PlayRound
├── docs/                  # rapport d'audit et protocole de mesure
└── constitution.md        # gouvernance technique des assistants IA
```

---

## 10. Feuille de route d'optimisation

Chaque palier fait l'objet d'une branche, d'une mesure isolée et d'une entrée
au journal d'optimisation.

1. Carte sur 1 octet, sabot `[208]uint8` distribué par curseur
2. Mains en tableaux fixes, suppression de toute allocation du hot path
3. Réordonnancement des champs d'état (`go vet -fieldalignment`)
4. Table de stratégie plate indexée arithmétiquement, sans hachage
5. Worker pool borné aux coeurs physiques, générateur par worker
6. Arrêt précoce sur critère d'intervalle de confiance
7. API binaire et persistance indexée des résultats

L'invariant est absolu : l'avantage de la maison mesuré ne doit pas bouger.
