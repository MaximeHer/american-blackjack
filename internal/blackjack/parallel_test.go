package blackjack

import (
	"context"
	"math"
	"runtime"
	"testing"
	"time"
)

// TestParallelDeterminisme vérifie qu'à graine et nombre de workers égaux, deux
// exécutions parallèles produisent des statistiques RIGOUREUSEMENT identiques.
//
// C'est la condition de validité du protocole de mesure sous parallélisme. Elle
// repose sur deux choix : des graines dérivées par worker, et une fusion dans
// l'ordre des indices et non dans l'ordre d'arrivée — l'addition flottante
// n'étant pas associative, agréger dans l'ordre d'achèvement rendrait le
// résultat dépendant de l'ordonnanceur.
func TestParallelDeterminisme(t *testing.T) {
	r := DefaultRules()
	ctx := context.Background()

	for _, workers := range []int{2, 4, runtime.NumCPU()} {
		a := SimulateParallel(ctx, 200_000, 42, r, 1, SideBets{}, workers)
		b := SimulateParallel(ctx, 200_000, 42, r, 1, SideBets{}, workers)
		if a != b {
			t.Fatalf("%d workers : deux exécutions de même graine diffèrent", workers)
		}
	}
}

// TestParallelCoherence vérifie que la parallélisation ne perd ni ne duplique
// aucun coup, quel que soit le découpage.
//
// Le reste à diviser est réparti sur les premiers workers : la somme des lots
// doit valoir exactement n, même quand n n'est pas divisible par le nombre de
// workers.
func TestParallelCoherence(t *testing.T) {
	r := DefaultRules()
	ctx := context.Background()

	// 100 003 est premier : aucun nombre de workers ne le divise.
	const n = 100_003
	for _, workers := range []int{1, 3, 7, 12, 16} {
		st := SimulateParallel(ctx, n, 7, r, 1, SideBets{}, workers)
		if st.Rounds != n {
			t.Errorf("%d workers : %d coups joués, %d attendus", workers, st.Rounds, n)
		}
		if st.MainWagered != float64(n) {
			t.Errorf("%d workers : mise totale %.0f, %d attendue", workers, st.MainWagered, n)
		}
	}
}

// TestParallelOracle vérifie que la version parallèle converge vers la même
// valeur publiée que la version séquentielle.
//
// Les deux ne peuvent PAS donner le même chiffre au bit près : chaque worker
// tire son propre sabot depuis une graine dérivée, donc l'échantillon diffère.
// Ce qui doit tenir, c'est la convergence statistique.
func TestParallelOracle(t *testing.T) {
	r := DefaultRules()
	ctx := context.Background()
	const n = 2_000_000

	st := SimulateParallel(ctx, n, 42, r, 1, SideBets{}, runtime.NumCPU())

	edge := st.HouseEdge() * 100
	stderr := st.StdError() * 100
	ecart := math.Abs(edge - expectedHouseEdge)

	t.Logf("avantage de la maison : %+.4f %% (erreur-type %.4f point)", edge, stderr)
	t.Logf("écart au publié       : %.4f point, soit %.2f erreurs-types", ecart, ecart/stderr)
	t.Logf("écart-type par coup   : %.4f", st.StdDev())

	if ecart > 4*stderr {
		t.Fatalf("la version parallèle dérive de %.2f erreurs-types du publié", ecart/stderr)
	}
	// L'écart-type par coup est une propriété du jeu, pas du découpage.
	if math.Abs(st.StdDev()-1.14) > 0.05 {
		t.Errorf("écart-type par coup = %.4f, attendu ~1,14", st.StdDev())
	}
}

// TestParallelAnnulation vérifie que l'annulation du contexte interrompt
// réellement les workers, sans attendre la fin du travail prévu.
func TestParallelAnnulation(t *testing.T) {
	r := DefaultRules()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // annulé avant même de commencer

	start := time.Now()
	st := SimulateParallel(ctx, 50_000_000, 42, r, 1, SideBets{}, runtime.NumCPU())
	elapsed := time.Since(start)

	// 50 millions de coups demanderaient plusieurs secondes ; l'annulation
	// doit couper bien avant.
	if elapsed > 2*time.Second {
		t.Errorf("l'annulation n'a pas interrompu les workers : %s écoulées", elapsed)
	}
	t.Logf("annulation honorée en %s, %d coups joués avant arrêt", elapsed.Round(time.Millisecond), st.Rounds)
}

// TestDeriveSeedDecorrelation vérifie que des indices de worker voisins ne
// produisent pas de graines voisines.
//
// Un simple `seed + w` donnerait à des workers consécutifs des séquences
// pseudo-aléatoires corrélées, ce qui biaiserait l'échantillon global sans
// qu'aucun test fonctionnel ne le signale.
func TestDeriveSeedDecorrelation(t *testing.T) {
	seen := map[int64]int{}
	for w := 0; w < 64; w++ {
		s := deriveSeed(42, w)
		if s < 0 {
			t.Errorf("worker %d : graine négative %d", w, s)
		}
		if prev, dup := seen[s]; dup {
			t.Errorf("workers %d et %d partagent la graine %d", prev, w, s)
		}
		seen[s] = w
	}

	// Deux indices consécutifs doivent donner des graines très éloignées.
	a, b := deriveSeed(42, 0), deriveSeed(42, 1)
	if d := a - b; d > -1_000_000 && d < 1_000_000 {
		t.Errorf("graines voisines trop proches : %d et %d", a, b)
	}
}

// BenchmarkSimulateParallel mesure l'accélération en fonction du nombre de
// workers, afin de vérifier la linéarité du passage à l'échelle.
func BenchmarkSimulateParallel(b *testing.B) {
	r := DefaultRules()
	ctx := context.Background()
	var st Stats

	for _, workers := range []int{1, 2, 4, 6, 8, 12} {
		b.Run(name(workers), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				st = SimulateParallel(ctx, 200_000, 42, r, 1, SideBets{}, workers)
			}
			sinkStats = st
		})
	}
}

func name(w int) string {
	if w < 10 {
		return "workers0" + string(rune('0'+w))
	}
	return "workers" + string(rune('0'+w/10)) + string(rune('0'+w%10))
}
