# Grille d'évaluation et plan de travail

Suivi de la couverture des critères du TP par les livrables du dépôt.

> **Rappel structurant** : le code n'est pas noté. Il sert de pièce à
> conviction pour prouver la reproductibilité et la véracité des mesures. Seul
> le rapport d'audit final fait foi. Chaque heure passée sur le code doit donc
> produire une mesure ou une preuve exploitable dans le rapport.

---

## Critère 1 — Environnement & métrologie (3 pts)

**Attendu** : spécification du banc d'essai (CPU, coeurs/threads, hiérarchie
L1/L2/L3, RAM, OS, version exacte du runtime) et protocole Hyperfine rigoureux
(warmup, itérations, isolation du bruit, moyenne/médiane/écart-type/variance).

| Élément | État |
|---|---|
| Relevé matériel automatisé, caches L1/L2/L3 inclus | à faire |
| Version exacte du runtime figée | partiel — `go 1.26` déclaré, toolchain à épingler |
| Protocole Hyperfine avec `--warmup` et itérations | à faire |
| Isolation du bruit documentée | à faire |
| Écart-type et variance | **acquis** — produits par le moteur (`Stats.StdDev`, `Stats.Variance`, `Stats.StdError`) |
| Benchmarks Go isolés par étage | **acquis** — `bench_test.go`, 8 benchmarks, mesure de référence dans `bench/baseline.txt` |
| Tailles de structures et remplissage | **acquis** — `TestStructSizes` |

**Point fort à exploiter** : le projet justifie physiquement son besoin de
performance. L'écart-type par coup (1,14) dépassant l'avantage mesuré (0,35 %),
l'erreur-type en `1/√n` impose ~1,3 × 10⁸ coups pour une résolution de
0,01 point. Le débit conditionne la précision statistique — c'est un argument
de métrologie, pas de confort.

---

## Critère 2 — Diagnostic & profiling réel (5 pts)

**Attendu** : flamegraphs et profils pprof annotés, générés sur la machine, et
identification formelle du hot path.

| Élément | État |
|---|---|
| Profil CPU (`-cpuprofile`) | à faire |
| Profil d'allocations (`-memprofile`, `-benchmem`) | à faire |
| Flamegraph annoté | à faire |
| Analyse textuelle du goulot | à faire |

**Suspects désignés par les benchmarks, à confirmer par le profil** :

| Mesure | Valeur | Suspect |
|---|---|---|
| `PlayRound` | 5 200 ns, 1 656 o, **46 allocs** | le journal narratif (~10 `Sprintf`) et les cartes allouées une par une |
| `Shuffle` | 19 300 ns, 15 488 o, **220 allocs** | mélange quadratique + 208 cartes allouées |
| `Decide` | 264 ns, **3 allocs** | `fmt.Sprintf` pour la clé, puis hachage de `map` |
| `Simulate` 10⁴ coups | **16,5 Mo, 469 801 allocs** | pression ramasse-miettes : 1,65 Go par million de coups |

Ne pas présupposer l'ordre d'importance : le profil CPU et le profil
d'allocations tranchent.

---

## Critère 3 — Journal d'optimisation (5 pts)

**Attendu** : justification théorique et physique des gains sur trois axes.

> Les 18 paliers sont détaillés un par un dans
> [01-perspectives-optimisation.md](01-perspectives-optimisation.md), chacun
> avec son hypothèse d'impact matériel, sa commande de vérification et sa
> condition de réfutation.

### Mémoire & localité de cache

| Palier | Cible | État |
|---|---|---|
| Journal narratif rendu explicite | retire ~10 `Sprintf` du chemin de simulation | à faire |
| `map[string]int` → tableau pour la valeur des cartes | supprime un hachage par carte | à faire |
| `[]*Card` → `[]Card` | supprime une allocation par carte, rétablit la localité | à faire |
| Carte sur 1 octet (4 bits rang + 2 bits enseigne) | `Card` 32 o → 1 o, facteur 32 | à faire |
| Sabot en tableau fixe + curseur | supprime 220 allocs par rebattage | à faire |
| Mélange de Fisher-Yates en place | quadratique → linéaire | à faire |
| Total de la main en incrémental | supprime le parcours répété | à faire |
| Réordonnancement des champs | récupère 19 o sur `Hand`, 28 o sur `RoundResult` | à faire |
| Mains en tableaux fixes | `0 allocs/op` sur `PlayRound` | à faire |
| Stratégie dévirtualisée + table plate | supprime appel dynamique, `Sprintf` et hachage | à faire |

### Concurrence & scalabilité CPU

| Palier | Cible | État |
|---|---|---|
| Worker pool borné à `NumCPU()` | 12 coeurs logiques disponibles, 1 utilisé | à faire |
| Générateur par worker dérivé d'une graine maître | conserve le déterminisme sous parallélisme | à faire |
| Agrégation atomique ou fusion locale | évite la contention | à faire |
| Arrêt précoce sur intervalle de confiance | cesse de simuler dès la précision atteinte | à faire |

### I/O réseau & persistance

| Palier | Cible | État |
|---|---|---|
| API binaire (Protobuf/gRPC) pour lancer une simulation | comparaison sérialisation vs JSON | à faire |
| Persistance indexée des résultats par configuration | `EXPLAIN ANALYZE` sur la recherche | à faire |
| Cache LRU des configurations déjà simulées | évite de recalculer | à faire |

C'est l'axe le plus faible du sujet : un simulateur n'a pas besoin
intrinsèquement d'une base de données. L'angle retenu est celui du **cache de
résultats précalculés** par jeu de règles, ce qui est défendable et utile.

---

## Critère 4 — Échec constructif (3 pts)

**Attendu** : au moins une tentative contre-productive documentée, avec analyse
chiffrée de la régression avant retour arrière.

Trois candidats, qui correspondent exactement aux trois exemples de l'énoncé :

1. **Parallélisation prématurée** — une goroutine par coup au lieu d'un worker
   pool par sabot. Le coût de création et de changement de contexte dépasse le
   travail utile d'un coup (~2 µs).
2. **Générateur aléatoire partagé** — conserver `math/rand` global entre
   plusieurs goroutines. Son mutex sérialise l'accès : la version parallèle
   peut devenir plus lente que la séquentielle. C'est l'échec le plus
   instructif, car il est involontaire et contre-intuitif.
3. **Cache trop gourmand** — mémoïser les décisions dans une map non bornée
   fait croître le tas et augmente la pression du ramasse-miettes.

Candidat retenu en priorité : le **n°2**, mesuré et expliqué mécaniquement,
avec les autres mentionnés. Les quatre échecs sont décrits avec leur mécanisme
et la commande de profilage qui les révèle dans
[01-perspectives-optimisation.md](01-perspectives-optimisation.md).

**Point d'ordonnancement** : l'échec n°2 doit être tenté **avant** la
parallélisation correcte. Constater la régression puis appliquer la dérivation
de graines est bien plus démonstratif que l'inverse, et c'est l'ordre dans
lequel on rencontre réellement le problème.

Chaque tentative vit sur une branche conservée, non fusionnée. Une branche
d'échec préservée est une preuve matérielle.

---

## Critère 5 — Reproductibilité & synthèse (4 pts)

**Attendu** : automatisation en une commande et tableau comparatif chiffrant
les gains d'ordres de grandeur.

| Élément | État |
|---|---|
| `Makefile` ou `run_benchmarks.sh` en une commande | à faire |
| Relevé matériel intégré au script | à faire |
| Comparaison par `benchstat` entre paliers | à faire |
| Mesures de bout en bout par `hyperfine` | à faire |
| Tableau de synthèse baseline vs final | à faire |

Le drapeau `-quiet` de la commande `simulate` n'émet que le débit, pour être
consommé directement par `hyperfine`.

---

## Critère 6 — Bonus `constitution.md` (+2 pts)

| Directive | État |
|---|---|
| Rôle et posture système stricts | **acquis** |
| Contraintes négatives explicites | **acquis** |
| Justification empirique (hypothèse / vérification) | **acquis** |
| Formatage compact et impératif | **acquis** |

Voir [constitution.md](../constitution.md).

---

## Point de contrôle permanent

Avant chaque fusion, sans exception :

```bash
go test ./... -v     # l'oracle doit passer
```

Un avantage de la maison qui dérive au-delà de 4 erreurs-types signifie que
l'optimisation a cassé la logique de jeu. Elle est annulée.
