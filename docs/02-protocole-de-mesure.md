# Protocole de mesure et banc d'essai

Spécification du matériel, conditions d'exécution, et protocole statistique
appliqué à toutes les mesures du projet.

Ce document répond au **critère 1** de la grille. Il contient aussi les erreurs
de protocole rencontrées et corrigées : elles sont instructives, et les taire
donnerait une fausse impression de rigueur.

---

## 1. Banc d'essai

### Processeur

| Caractéristique | Valeur |
|---|---|
| Modèle | AMD Ryzen 5 5600H with Radeon Graphics |
| Architecture | Zen 3 |
| Cœurs physiques | **6** |
| Cœurs logiques | **12** (SMT 2 voies) |
| Fréquence nominale | 3 301 MHz |

### Hiérarchie de cache

Relevée par `Get-CimInstance Win32_CacheMemory`. Les niveaux WMI sont décalés
de 2 : `Level=3` désigne le L1, `Level=4` le L2, `Level=5` le L3.

| Niveau | Taille totale | Par cœur | Partage |
|---|---|---|---|
| **L1** | 384 Ko | 64 Ko (32 Ko I + 32 Ko D) | privé par cœur |
| **L2** | 3 072 Ko | 512 Ko | privé par cœur |
| **L3** | 16 384 Ko | — | partagé par les 6 cœurs |

**Ligne de cache : 64 octets.** C'est la granularité qui gouverne tout le
travail de localité : une structure de 32 octets en tient 2 par ligne, une de
1 octet en tient 64.

Repère dimensionnant pour ce projet : un sabot de 4 jeux occupe **6 656 octets**
dans la version de référence (208 cartes × 32 o), soit **104 lignes de cache**.
Compacté à un octet par carte il occuperait **208 octets**, soit **4 lignes**, et
tiendrait intégralement dans le L1.

### Mémoire

| Caractéristique | Valeur |
|---|---|
| Capacité | 16 Go (15,3 Go utilisables) |
| Fréquence | 3 200 MT/s |
| Modules | **1 seule barrette** (Samsung M471A2K43EB1-CWE) |
| Configuration | **simple canal** |

> **Limite à signaler.** Une seule barrette signifie un fonctionnement en
> simple canal, qui **divise par deux la bande passante mémoire** disponible par
> rapport à une configuration à deux barrettes. Pour une charge limitée par la
> mémoire — ce que la version de référence est, avec 519 Mo/s d'allocation —
> c'est une contrainte matérielle réelle. Les gains de localité mesurés sur
> cette machine seront donc *supérieurs* à ceux qu'on observerait en double
> canal, puisque chaque défaut de cache y coûte plus cher.

### Système et runtime

| Caractéristique | Valeur |
|---|---|
| Système | Microsoft Windows 11 Famille |
| Build | 26200 (version 10.0.26200) |
| Runtime Go | **go1.26.4** windows/amd64 |
| GOMAXPROCS | 12 (valeur par défaut) |

---

## 2. Conditions d'exécution

Toute mesure rapportée est prise dans ces conditions, vérifiées avant chaque
campagne :

- **Machine sur secteur**, batterie à 100 % (`Win32_Battery.BatteryStatus = 2`)
- **Machine au repos** : charge CPU sous 5 % avant lancement
- Aucune compilation, aucun test et aucun benchmark concurrent
- Processeur à sa fréquence nominale, non throttlé thermiquement

### Pourquoi ces précautions ne sont pas facultatives

Le Ryzen 5 5600H est un **processeur mobile**, dont la fréquence dépend
fortement de l'alimentation, de la température et de la charge. Une mesure prise
dans de mauvaises conditions ne se contente pas d'être imprécise : elle est
fausse d'un facteur qui dépasse largement les gains que l'on cherche à mesurer.

Vérification des conditions :

```powershell
(Get-CimInstance Win32_Battery).BatteryStatus          # 2 = secteur
(Get-CimInstance Win32_Processor).LoadPercentage        # doit être < 5
(Get-CimInstance Win32_Processor).CurrentClockSpeed     # doit être ~3301
```

---

## 3. Protocoles

### 3.1 Mesure de bout en bout

Pour le débit global du moteur.

```bash
go build -o simulate ./cmd/simulate
for i in $(seq 1 15); do ./simulate -rounds 500000 -warmup 50000 -quiet; done
```

**15 exécutions**, dont on rapporte la **médiane** et l'**écart-type**. Jamais
une valeur unique.

La chauffe de 50 000 coups a été mesurée comme **sans effet significatif**
(356 251 contre 351 652 coups/s, soit en deçà de la dispersion). Elle est
conservée par principe, non par nécessité démontrée.

Le drapeau `-quiet` n'émet que le débit, ce qui rend la commande directement
consommable par un script ou par `hyperfine`.

### 3.1 bis Mesure de bout en bout par hyperfine

```bash
hyperfine --shell=none --warmup 3 --runs 15   --export-markdown resultat.md --export-json resultat.json   './simulate -rounds 500000 -quiet'
```

Résultat sur la version courante : **621,8 ms ± 12,5 ms** pour 500 000 coups,
étendue 599,8 – 639,1 ms, soit **804 000 coups/s** avec un écart-type de
**2,0 %** — la meilleure dispersion obtenue sur ce projet.

Trois décisions de protocole, chacune pour une raison précise.

**`--shell=none`.** Sans cette option, hyperfine passe par `cmd.exe` sous
Windows, ce qui a d'abord provoqué un échec — `cmd.exe` refuse un chemin relatif
à barres obliques. Mais c'est surtout meilleur méthodologiquement : exécuter le
binaire directement **retire le lancement du shell du temps mesuré**, qui n'a
rien à voir avec ce qu'on cherche à chiffrer.

**Pas de `-warmup` du moteur sous hyperfine.** Les deux chauffes se cumulaient :
les coups de chauffe internes entraient dans le temps mesuré, donc le temps
rapporté ne se divisait pas proprement par le nombre de coups annoncé. La chauffe
est désormais assurée au seul niveau du processus, par `--warmup 3`.

**`--runs 15`.** Même raisonnement qu'à la section 3.1 : une mesure unique sur ce
matériel ne vaut rien.

### 3.2 Micro-benchmarks

Pour attribuer un gain à un changement précis.

```bash
go test -run '^$' -bench . -benchmem -benchtime 3s -count 12 \
  ./internal/blackjack > bench/avant.txt
# ... une seule optimisation ...
go test -run '^$' -bench . -benchmem -benchtime 3s -count 12 \
  ./internal/blackjack > bench/apres.txt
benchstat bench/avant.txt bench/apres.txt
```

Règles appliquées :

1. **Un changement à la fois**, un commit par optimisation.
2. **Comparaison par `benchstat`**, jamais deux nombres bruts. Un écart
   inférieur à la dispersion n'est pas un gain.
3. **Variance cible sous 5 %.** Au-delà, la mesure est rejetée et relancée.

### 3.3 Comptage d'opérations

Pour expliquer d'où vient le travail, jamais pour mesurer un débit.

```bash
go run -tags instrument ./cmd/simulate -rounds 200000
```

Les compteurs sont **absents du binaire par défaut** : `Instrumented` est une
constante fausse, donc tout bloc de comptage est éliminé à la compilation.

Preuve par le code machine :

```bash
go build -o def.exe ./cmd/simulate
go build -tags instrument -o ins.exe ./cmd/simulate
go tool objdump -s 'blackjack\.\(\*Hand\)\.Total' def.exe | wc -l   # 0
go tool objdump -s 'blackjack\.\(\*Hand\)\.Total' ins.exe | wc -l   # 77
go tool nm def.exe | grep -c countHandTotal                          # 0
```

Dans le binaire par défaut, `Hand.Total` **n'existe pas comme fonction** : elle
est entièrement inlinée et aucun symbole de comptage ne subsiste. Dans le
binaire instrumenté elle redevient une fonction autonome de 77 instructions,
car l'appel atomique empêche l'inlining.

**Le coût de l'instrumentation n'est donc pas une incrémentation : c'est la
perte de l'inlining.** C'est pour cette raison qu'elle doit être compilée hors
du binaire de mesure, et non simplement « laissée là, ça ne coûte rien ».

### 3.4 Métriques du runtime

Toutes les grandeurs mémoire, GC et ordonnanceur sont lues par
`internal/metrics`, **avant et après** la boucle, jamais pendant. Le coût est
donc constant : deux appels à `runtime/metrics.Read`.

Limite connue et documentée : `/gc/heap/allocs:objects` est alimenté par des
caches propres à chaque processeur logique, vidés paresseusement. Le retard
mesuré est de **5 objets sur 2 000**, soit 0,25 %. C'est un petit nombre
*absolu*, borné par le nombre de processeurs : négligeable sur 23 millions
d'objets, sensible sur quelques milliers. Pour un comptage exact sur une petite
charge, `go test -benchmem` force un vidage.

---

## 4. Erreurs de protocole rencontrées

### 4.1 Une mesure unique sur machine chargée : facteur 1,9

**Première mesure rapportée : 181 112 coups/s.** Mesure correcte après
correction : **342 306 coups/s**. Un facteur **1,9** sur du code identique.

Cause : la mesure initiale avait été prise immédiatement après l'exécution de la
suite de tests et des benchmarks, sur une machine encore chargée et
thermiquement sollicitée.

Ce qui est en jeu : un facteur 1,9 de bruit sur un projet dont certains paliers
d'optimisation visent 20 à 30 % de gain. **Le bruit aurait dépassé le signal.**
Toute conclusion tirée de mesures uniques aurait été sans valeur.

Correctif appliqué : 15 exécutions, médiane et écart-type, conditions vérifiées.

### 4.2 Échantillons trop courts sur les benchmarks amortis

À `-benchtime 1s`, quatre benchmarks dépassaient les 5 % de variance. Le
rebattage survenant par à-coups tous les ~29 coups, son coût amorti dépend du
nombre de rebattages tombant dans l'échantillon.

| Benchmark | ± à 1 s | ± à 3 s |
|---|---|---|
| `PlayRound` | 7 % | **3 %** |
| `PlayRoundSideBets` | 18 % | **2 %** |
| `Shuffle` | 12 % | 16 % |
| `Deal` | 14 % | 11 % |

Correctif : `-benchtime 3s`, qui ramène les deux premiers sous le seuil.

### 4.3 La variance résiduelle vient du ramasse-miettes

`Shuffle` et `Deal` restaient bruités après allongement. Hypothèse : ce sont les
plus gourmands en allocations, et les cycles de GC tombent de façon
imprévisible. Vérification en désactivant le GC :

| Benchmark | GC actif | `GOGC=off` | Écart |
|---|---|---|---|
| `Shuffle` | 15,97 µs ± 16 % | **11,64 µs ± 10 %** | **−27 %** |
| `Deal` | 85,76 ns ± 9 % | **62,00 ns ± 7 %** | **−28 %** |

Hypothèse confirmée et chiffrée. Le ramasse-miettes représente **27 % du temps**
sur les chemins riches en allocations, et il injecte une part de la variance.

Deux conséquences :

1. **Argument direct pour la campagne zéro-allocation.** Ce n'est pas une
   question de pureté stylistique : 27 % du temps est mesurable et attribuable.
2. **La version de référence est partiellement non mesurable à cause de ses
   propres déchets.** La stabilisation des benchmarks après suppression des
   allocations sera donc un résultat en soi, et non un simple effet de bord.

`GOGC=off` est un **outil de diagnostic, pas une solution** : sans ramassage, le
tas croît indéfiniment jusqu'à saturation.

---

## 5. Mesure de référence

Relevée dans les conditions de la section 2, le 2026-10-07.

### Bout en bout, 15 exécutions de 500 000 coups

| Grandeur | Valeur |
|---|---|
| Débit médian | **342 306 coups/s** |
| Temps par coup | **2 921 ns** |
| Moyenne | 339 800 coups/s |
| Écart-type | 15 363 coups/s — **4,52 %** |
| Variance | 236 023 751 |
| Min – max | 297 537 – 355 637 coups/s |
| Étendue | 17 % de la médiane |

### Micro-benchmarks, `-benchtime 3s -count 12`

| Benchmark | Temps | Mémoire | Allocations | ± |
|---|---|---|---|---|
| `PlayRound` | 3,10 µs | 1 656 o | **46** | 3 % |
| `PlayRoundSideBets` | 4,34 µs | 2 081 o | **57** | 2 % |
| `Shuffle` | 20,6 µs | 15 488 o | **220** | 16 % |
| `Deal` | 112 ns | 74 o | 1 | 14 % |
| `HandTotal` | 29,9 ns | 0 | 0 | 2 % |
| `Decide` | 208 ns | 48 o | 3 | 2 % |
| `SideBetEval` | 84,0 ns | 0 | 0 | 2 % |
| `Simulate` 10⁴ coups | 34,1 ms | 15,8 Mio | **469 801** | 5 % |

### Comportement mémoire et GC, sur 500 000 coups

| Grandeur | Valeur |
|---|---|
| Alloué | 790 Mio — **1 657 o par coup** |
| Objets alloués | 22 998 051 — **46,0 par coup** |
| Débit d'allocation | **519 Mo/s** |
| Cycles de GC | 244 — 488 par million de coups |
| Pause GC cumulée | 26,75 ms — 1,84 % du temps horloge |
| Pause GC p50 / p99 / max | 64 ns / 1,05 ms / 14,68 ms |
| **Part CPU du ramasse-miettes** | **15,25 %** |
| Parallélisme effectif | **1,32 cœur** sur 12 |

> **Lecture du parallélisme.** Le moteur est strictement mono-thread, et
> consomme pourtant 1,32 cœur de temps CPU. Les 0,32 cœur excédentaires sont le
> ramasse-miettes, qui travaille en parallèle sur les autres cœurs. La pression
> d'allocation ne ralentit donc pas seulement la boucle : elle occupe des cœurs
> qui pourraient servir à simuler.

### Opérations élémentaires, binaire instrumenté, 200 000 coups

| Opération | Par coup | Lecture |
|---|---|---|
| Décisions | 1,33 | — |
| **Appels à `Total()`** | **10,38** | soit **7,81 par décision** : la redondance est mesurée, non supposée |
| Évaluations de carte | 29,17 | chacune étant une consultation de map |
| Consultations de map | 30,50 | dont 1,00 par décision pour la table de stratégie |
| **Déplacements de mélange** | — | **10 756 par rebattage** |

Le comptage des déplacements **confirme la prédiction théorique**. L'index tiré
étant uniforme, le nombre moyen de déplacements vaut n(n−1)/4, soit
208 × 207 / 4 = **10 764**. Mesuré : **10 756**. Fisher-Yates ferait le même
travail en 208 échanges, soit un **facteur 52**.

> Une estimation antérieure avançait ~21 600 déplacements : elle calculait
> n(n−1)/2, en oubliant que l'index tiré se situe en moyenne au milieu du
> paquet et non à son début. L'écart avec la mesure a permis de retrouver
> l'erreur — c'est précisément à cela que sert un compteur.

### Tailles des structures

Relevées par `unsafe.Sizeof` dans `TestStructSizes`.

| Structure | Taille | Champs utiles | Remplissage | Par ligne de 64 o |
|---|---|---|---|---|
| `Card` | 32 o | 32 o | 0 o | 2 |
| `Hand` | 56 o | 37 o | **19 o (34 %)** | 1 |
| `RoundResult` | 104 o | 76 o | **28 o (27 %)** | — |

---

## 6. Le harnais, en une commande

```bash
bash scripts/run_benchmarks.sh
```

Produit l'intégralité du dossier de mesure dans `results/<horodatage>/` :

| Fichier | Contenu |
|---|---|
| `00-banc-essai.txt` | relevé matériel complet, conditions vérifiées |
| `10-oracle.txt` | oracle de non-régression |
| `20-bench.txt`, `21`, `22` | benchmarks, dispersion, comparaison au tag `v0-baseline` |
| `30`–`32-hyperfine.*` | mesure de bout en bout, en markdown et JSON |
| `33`–`34-metriques.*` | débit, mémoire, GC et ordonnanceur du moteur |
| `35-operations.txt` | compteurs d'opérations, binaire instrumenté séparé |
| `41`–`44-*` | profils CPU et allocations, annotations ligne par ligne |
| `99-resume.md` | résumé citable, avec la révision Git mesurée |

Étapes isolables : `hardware`, `test`, `bench`, `hyperfine`, `profile`.

Paramétrable par variables d'environnement : `ROUNDS`, `BENCH_TIME`,
`BENCH_COUNT`, `HF_RUNS`, `HF_WARMUP`.

### Deux refus délibérés

**Le harnais refuse de mesurer sur batterie.** Il interrompt la campagne avec un
message explicite. Une mesure sur batterie n'est pas imprécise, elle est fausse
— voir la section 4.1 et son facteur 1,9.

**Le harnais s'arrête si l'oracle échoue**, avant même de lancer le moindre
benchmark. Chiffrer les performances d'un moteur dont la logique est cassée n'a
aucun sens, et l'ordre des étapes le fait respecter mécaniquement.

Le résumé consigne la révision Git mesurée et signale un arbre de travail
modifié, afin qu'aucun chiffre ne puisse être cité sans savoir à quel état du
code il correspond.

### Un `Makefile` en complément

`make` n'est pas installé sur la machine de développement. Le script bash est
donc le point d'entrée canonique. Un `Makefile` est fourni pour les
environnements qui ont `make`, et sert de table des matières des commandes :
`make measure`, `make check`, `make oracle`, `make profile-lines`, `make flame`.

---

## 7. Reste à mettre en place

| Élément | État |
|---|---|
| Relevé matériel automatisé | **acquis** — `scripts/hardware.ps1` |
| Protocole `hyperfine --warmup --runs` | **acquis** — § 3.1 bis |
| `run_benchmarks.sh` en une commande | **acquis** — § 6 |
| Profils CPU et allocations annotés | **acquis** — [03-profiling.md](03-profiling.md) |
| Flamegraph | commande documentée, capture à joindre au rapport |
| Tableau de synthèse final | à produire en fin de campagne (critère 5) |
