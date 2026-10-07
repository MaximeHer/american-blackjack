package metrics

import (
	"runtime"
	"testing"
	"time"
)

// TestReportCoherence vérifie qu'un rapport décrit bien la charge mesurée :
// grandeurs positives, dérivations cohérentes, et détection effective des
// allocations.
func TestReportCoherence(t *testing.T) {
	// La charge doit durer assez longtemps pour dépasser la résolution de
	// l'horloge monotone de la plateforme : sous Windows, quelques milliers
	// d'allocations s'exécutent en deçà du pas de l'horloge et la durée
	// mesurée ressort à zéro. 50 000 tranches de 512 octets, soit 25 Mio,
	// garantissent une mesure exploitable.
	const units = 50_000

	run := Begin()
	sink := make([][]byte, 0, units)
	for i := 0; i < units; i++ {
		sink = append(sink, make([]byte, 512))
	}
	r := run.End(units)
	runtime.KeepAlive(sink)

	if r.Units != units {
		t.Fatalf("Units = %d, attendu %d", r.Units, units)
	}
	if r.Wall <= 0 {
		t.Fatal("la durée horloge doit être positive")
	}
	if r.OpsPerS <= 0 {
		t.Fatal("le débit doit être positif")
	}
	if r.NsPerOp <= 0 {
		t.Fatal("le temps par unité doit être positif")
	}

	// 50 000 tranches de 512 octets représentent environ 25 Mio. La tolérance de
	// 1 % absorbe le retard des caches d'allocation par processeur, que le
	// runtime vide paresseusement : les derniers objets alloués peuvent ne pas
	// encore être comptés au moment de la lecture.
	if min := uint64(float64(units) * 512 * 0.99); r.BytesAlloc < min {
		t.Errorf("octets alloués = %d, au moins %d attendus", r.BytesAlloc, min)
	}
	if min := uint64(float64(units) * 0.99); r.ObjectsAlloc < min {
		t.Errorf("objets alloués = %d, au moins %d attendus", r.ObjectsAlloc, min)
	}

	// Cohérence des dérivations.
	if got, want := r.BytesPerOp, float64(r.BytesAlloc)/float64(units); got != want {
		t.Errorf("BytesPerOp = %v, attendu %v", got, want)
	}
	if got, want := r.AllocsPerOp, float64(r.ObjectsAlloc)/float64(units); got != want {
		t.Errorf("AllocsPerOp = %v, attendu %v", got, want)
	}

	if r.GOMAXPROCS <= 0 || r.NumCPU <= 0 {
		t.Error("l'environnement doit être renseigné")
	}
	if r.GoVersion == "" || r.GOOS == "" || r.GOARCH == "" {
		t.Error("la version du runtime et la plateforme doivent être renseignées")
	}
}

// TestZeroUnits vérifie qu'un rapport sur zéro unité ne divise pas par zéro.
func TestZeroUnits(t *testing.T) {
	r := Begin().End(0)
	if r.NsPerOp < 0 || r.BytesPerOp < 0 || r.AllocsPerOp < 0 {
		t.Fatal("aucune grandeur ne doit être négative")
	}
}

// TestHistogramesIsoles vérifie que la soustraction d'histogrammes isole bien
// l'intervalle mesuré : deux mesures successives ne doivent pas se cumuler.
//
// C'est le piège principal du paquet : metrics.Read réutilise ses tampons d'un
// appel à l'autre, et sans recopie la lecture initiale serait écrasée par la
// finale, ce qui rendrait toute différence nulle.
func TestHistogramesIsoles(t *testing.T) {
	first := Begin()
	runtime.GC()
	r1 := first.End(1)

	second := Begin()
	r2 := second.End(1)

	if r1.GCCycles == 0 {
		t.Fatal("un runtime.GC() explicite doit être compté comme un cycle")
	}
	if r2.GCCycles != 0 {
		t.Errorf("un intervalle sans GC doit compter 0 cycle, obtenu %d — "+
			"les histogrammes ou compteurs ne sont pas correctement isolés",
			r2.GCCycles)
	}
}

// TestQuantileEstimation vérifie le calcul de quantile sur un histogramme
// construit à la main, aux bornes incluses.
func TestQuantileEstimation(t *testing.T) {
	// Trois seaux : [0, 1s), [1s, 2s), [2s, 3s), avec 10 observations chacun.
	h := newTestHist([]float64{0, 1, 2, 3}, []uint64{10, 10, 10})

	cases := []struct {
		q    float64
		want time.Duration
	}{
		{0.10, 1 * time.Second},
		{0.50, 2 * time.Second},
		{0.99, 3 * time.Second},
	}
	for _, tc := range cases {
		if got := quantile(h, tc.q); got != tc.want {
			t.Errorf("quantile(%.2f) = %v, attendu %v", tc.q, got, tc.want)
		}
	}

	if got := histMax(h); got != 3*time.Second {
		t.Errorf("histMax = %v, attendu 3s", got)
	}
	if got := histCount(h); got != 30 {
		t.Errorf("histCount = %d, attendu 30", got)
	}

	// Un histogramme vide ne doit pas paniquer ni inventer de valeur.
	empty := newTestHist([]float64{0, 1}, []uint64{0})
	if got := quantile(empty, 0.5); got != 0 {
		t.Errorf("quantile sur histogramme vide = %v, attendu 0", got)
	}
	if got := quantile(nil, 0.5); got != 0 {
		t.Errorf("quantile(nil) = %v, attendu 0", got)
	}
}
