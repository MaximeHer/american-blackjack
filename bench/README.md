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

## Comparer deux états

```bash
benchstat bench/baseline.txt bench/palier-01.txt
```

Jamais deux nombres bruts. Un écart inférieur à la dispersion n'est pas un gain.

## Fichiers

| Fichier | État mesuré |
|---|---|
| `baseline.txt` | version de référence, tag `v0-baseline` |

Les fichiers de paliers sont ajoutés au fur et à mesure.
