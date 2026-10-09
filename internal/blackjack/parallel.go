package blackjack

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
)

// SimulateParallel joue n coups répartis sur un pool de workers borné.
//
// RANG 6 DU PROFIL. Jusqu'ici le moteur utilisait un cœur sur douze. La
// simulation est pourtant massivement parallélisable : deux sabots n'ont aucun
// état commun, aucune carte ne circule de l'un à l'autre.
//
// # Pool borné, jamais une goroutine par tâche
//
// Le pool est dimensionné sur runtime.NumCPU() et ne dépasse jamais cette
// borne. Lancer une goroutine par coup — la tentation naturelle — coûterait
// plus cher que le travail utile : un coup dure désormais 178 ns, là où la
// création et l'ordonnancement d'une goroutine se comptent en centaines de
// nanosecondes. Le pool amortit ce coût sur des millions de coups.
//
// La charge étant strictement CPU-bound — aucune I/O, aucune attente — le
// dimensionnement correct est le nombre de cœurs, pas un multiple.
//
// # Zéro partage pendant le calcul
//
// Chaque worker accumule ses statistiques dans une variable LOCALE, sur sa
// propre pile, et ne touche à la mémoire partagée qu'une seule fois, à la fin.
//
// Ce n'est pas un détail de style. Si les workers écrivaient directement dans
// un tableau partagé, deux compteurs voisins pourraient tomber sur la même
// ligne de cache de 64 octets : chaque écriture de l'un invaliderait la ligne
// dans le cache de l'autre, déclenchant un va-et-vient permanent du protocole
// de cohérence MESI. C'est le faux partage, et il annule l'essentiel du gain
// de parallélisme sans qu'aucun verrou ne soit en cause.
//
// Accumuler localement supprime le problème à la racine plutôt que de le
// contourner par du remplissage.
//
// # Déterminisme préservé
//
// Chaque worker reçoit une graine DÉRIVÉE de la graine maître, et la fusion
// finale se fait dans l'ordre des indices de worker, jamais dans l'ordre
// d'arrivée. Deux exécutions de même graine et de même nombre de workers
// produisent donc des statistiques rigoureusement identiques — ce que le
// protocole de mesure exige, et qu'une agrégation dans l'ordre d'achèvement
// ruinerait, l'addition flottante n'étant pas associative.
//
// Un ctx annulé interrompt les workers entre deux lots.
func SimulateParallel(ctx context.Context, n int, seed int64, r Rules, bet float64, sb SideBets, workers int) Stats {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}

	// Résultats indexés par worker : chacun n'écrit que sa propre case, et
	// une seule fois.
	parts := make([]Stats, workers)

	base, extra := n/workers, n%workers

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		// Les `extra` premiers workers prennent un coup de plus, afin que la
		// somme fasse exactement n sans qu'aucun worker ne soit à vide.
		count := base
		if w < extra {
			count++
		}

		go func(w, count int) {
			defer wg.Done()
			parts[w] = runChunk(ctx, count, deriveSeed(seed, w), r, bet, sb)
		}(w, count)
	}
	wg.Wait()

	// Fusion dans l'ordre des indices, jamais dans l'ordre d'arrivée.
	var total Stats
	for w := 0; w < workers; w++ {
		total.Merge(parts[w])
	}
	return total
}

// runChunk joue count coups sur un sabot privé.
//
// L'accumulation se fait dans une variable locale : aucune écriture en mémoire
// partagée avant le retour.
func runChunk(ctx context.Context, count int, seed int64, r Rules, bet float64, sb SideBets) Stats {
	rng := rand.New(rand.NewSource(seed))
	shoe := NewShoe(r.NumDecks, r.Penetration, rng)

	var st Stats
	for i := 0; i < count; i++ {
		// L'annulation est vérifiée par lots de 4 096 coups. La tester à
		// chaque coup ajouterait une lecture atomique sur un chemin qui dure
		// 178 ns, pour une réactivité dont personne n'a besoin.
		if i&0xFFF == 0 && ctx != nil {
			select {
			case <-ctx.Done():
				return st
			default:
			}
		}
		if shoe.CutReached() {
			shoe.Shuffle()
		}
		st.Add(PlayRound(shoe, r, bet, sb, nil))
	}
	st.Shuffles = shoe.Shuffles
	st.CardsDealt = shoe.CardsDealt
	return st
}

// deriveSeed fabrique la graine d'un worker à partir de la graine maître.
//
// Un simple `seed + w` donnerait à des workers voisins des séquences
// pseudo-aléatoires corrélées, ce qui biaiserait l'échantillon. Le mélange de
// bits ci-dessous — l'avalanche de SplitMix64 — disperse les graines voisines
// sur tout l'espace, de sorte que deux workers consécutifs produisent des
// séquences sans rapport.
func deriveSeed(master int64, worker int) int64 {
	x := uint64(master) + uint64(worker+1)*0x9E3779B97F4A7C15
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return int64(x >> 1) // positif, rand.NewSource n'accepte pas de négatif utile
}
