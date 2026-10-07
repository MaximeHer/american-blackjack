# Mesures de référence

## Protocole canonique

Toutes les mesures de ce dossier sont produites par la même commande, afin que
les comparaisons `benchstat` soient homogènes :

```bash
go test -run '^$' -bench . -benchmem -benchtime 2s -count 8 \
  ./internal/blackjack > bench/<nom>.txt
```

Conditions exigées, vérifiées avant chaque campagne : machine sur secteur, au
repos (charge CPU sous 5 %), aucune compilation ni test concurrent. Voir
[../docs/02-protocole-de-mesure.md](../docs/02-protocole-de-mesure.md).

`-benchtime 2s` n'est pas un détail de confort : à une seconde, quatre
benchmarks dépassaient les 5 % de variance que `constitution.md` impose, parce
que le coût du rebattage est amorti par à-coups.

## Puits de benchmark obligatoires

Tous les benchmarks accumulent leur résultat dans une variable locale affectée à
une variable de paquet après la boucle (`sinkInt`, `sinkFloat`, …).

Sans cela, le compilateur **élimine le calcul** dont le résultat n'est jamais lu
(*Dead Code Elimination*), et le benchmark mesure une boucle vide.

Mesuré sur ce projet : la surestimation atteignait **+20 % sur `PlayRound`** et
**+15 % sur `HandTotal`** — la distorsion est maximale sur les petites fonctions
inlinables, et nulle sur celles qui ont des effets de bord que le compilateur ne
peut pas supprimer (`Shuffle`, `Simulate`).

`baseline.txt` a été **remesurée a posteriori** avec des puits, afin que toute
comparaison porte sur une méthodologie identique. L'ancienne mesure est
conservée dans `baseline-sans-puits-ERRONE.txt` à titre de pièce à conviction.

## Comparer deux états

```bash
benchstat bench/baseline.txt bench/palier-01.txt
```

Jamais deux nombres bruts. Un écart inférieur à la dispersion n'est pas un gain.

## Fichiers

| Fichier | État mesuré |
|---|---|
| `baseline.txt` | tag `v0-baseline`, **avec puits** — la référence de toute comparaison |
| `baseline-sans-puits-ERRONE.txt` | même état, sans puits : conservé pour documenter l'erreur |
| `palier-01` à `palier-03` | paliers choisis avant profilage, mesurés sans puits |
| `rang-01-fisher-yates.txt` | mélange en place, sans puits |
| `rang-02-carte-1-octet.txt` | carte sur un octet, **avec puits** |

Seules les comparaisons entre fichiers de même méthodologie sont valides. La
comparaison qui fait foi pour le rapport est `baseline.txt` contre le dernier
palier, toutes deux avec puits.
