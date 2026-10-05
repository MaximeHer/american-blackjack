package blackjack

import (
	"math/rand"
	"testing"
	"unsafe"
)

// Ces benchmarks sont les points de mesure du travail d'optimisation. Chacun
// isole un étage du moteur pour que `benchstat` puisse attribuer un gain à un
// changement précis plutôt qu'à l'ensemble.
//
// Protocole : toujours comparer deux états par benchstat, jamais deux nombres
// bruts, et toujours avec -benchmem pour suivre les allocations.
//
//	go test -run '^$' -bench . -benchmem -count 10 ./internal/blackjack > bench/avant.txt
//	# ... une optimisation, une seule ...
//	go test -run '^$' -bench . -benchmem -count 10 ./internal/blackjack > bench/apres.txt
//	benchstat bench/avant.txt bench/apres.txt

// benchShoe fournit un sabot entretenu : il est rebattu dès que la coupe est
// atteinte, afin que le benchmark mesure un régime permanent et non un sabot
// qui s'épuise.
func benchShoe(r Rules, seed int64) *Shoe {
	return NewShoe(r.NumDecks, r.Penetration, rand.New(rand.NewSource(seed)))
}

// BenchmarkPlayRound est LA mesure de référence du projet : le coût d'un coup
// complet, sans paris annexes.
func BenchmarkPlayRound(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)
	var sb SideBets

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.CutReached() {
			s.Shuffle()
		}
		_ = PlayRound(s, r, 1, sb)
	}
}

// BenchmarkPlayRoundSideBets mesure le surcoût des quatre paris annexes, qui
// imposent de suivre les enseignes et d'évaluer des combinaisons de poker.
func BenchmarkPlayRoundSideBets(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)
	sb := SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.CutReached() {
			s.Shuffle()
		}
		_ = PlayRound(s, r, 1, sb)
	}
}

// BenchmarkShuffle isole le rebattage du sabot. La version de référence
// reconstruit 208 cartes allouées une par une, puis les mélange par un
// algorithme quadratique.
func BenchmarkShuffle(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Shuffle()
	}
}

// BenchmarkDeal isole la distribution d'une carte.
func BenchmarkDeal(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.Remaining() == 0 {
			s.Shuffle()
		}
		_ = s.Deal()
	}
}

// BenchmarkHandTotal isole le calcul du total d'une main. La fonction est
// appelée plusieurs fois par décision, ce qui en fait la plus chaude du
// moteur.
func BenchmarkHandTotal(b *testing.B) {
	h := &Hand{Cards: []*Card{
		{Rank: "A", Suit: "Pique"},
		{Rank: "7", Suit: "Coeur"},
		{Rank: "5", Suit: "Carreau"},
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Total()
	}
}

// BenchmarkDecide isole la consultation de la stratégie de base : construction
// d'une clé textuelle, hachage, et appel par interface.
func BenchmarkDecide(b *testing.B) {
	r := DefaultRules()
	h := &Hand{Cards: []*Card{
		{Rank: "10", Suit: "Pique"},
		{Rank: "6", Suit: "Coeur"},
	}}
	up := &Card{Rank: "9", Suit: "Trefle"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultStrategy.Decide(h, up, r, 1)
	}
}

// BenchmarkSideBetEval isole l'évaluation des paris annexes, dont le 21+3 qui
// trie trois cartes pour détecter une suite.
func BenchmarkSideBetEval(b *testing.B) {
	a := &Card{Rank: "5", Suit: "Pique"}
	c2 := &Card{Rank: "6", Suit: "Pique"}
	up := &Card{Rank: "7", Suit: "Pique"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EvalPerfectPairs(a, c2)
		_, _ = EvalTwentyOnePlus3(a, c2, up)
		_, _ = EvalLuckyLadies(a, c2, false)
	}
}

// BenchmarkSimulate mesure la boucle complète sur un nombre fixe de coups.
// C'est l'équivalent interne de la mesure de bout en bout faite par hyperfine
// sur le binaire.
func BenchmarkSimulate(b *testing.B) {
	r := DefaultRules()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Simulate(10_000, 42, r, 1, SideBets{})
	}
}

// TestStructSizes documente le coût mémoire des structures du moteur.
//
// Il n'échoue jamais : il produit les chiffres à citer dans le rapport. Deux
// gaspillages y sont visibles — la représentation elle-même (deux chaînes pour
// une carte qui tiendrait dans un octet) et le remplissage inséré par le
// compilateur, Go ne réordonnant jamais les champs d'une structure.
func TestStructSizes(t *testing.T) {
	type row struct {
		name   string
		size   uintptr
		fields uintptr // somme des tailles des champs, hors remplissage
	}

	var c Card
	var h Hand
	var rr RoundResult
	var sh Shoe

	rows := []row{
		{"Card", unsafe.Sizeof(c),
			unsafe.Sizeof(c.Rank) + unsafe.Sizeof(c.Suit)},
		{"Hand", unsafe.Sizeof(h),
			unsafe.Sizeof(h.Doubled) + unsafe.Sizeof(h.Bet) + unsafe.Sizeof(h.FromSplit) +
				unsafe.Sizeof(h.Cards) + unsafe.Sizeof(h.SplitAce) + unsafe.Sizeof(h.Stood) +
				unsafe.Sizeof(h.Surrendered)},
		{"RoundResult", unsafe.Sizeof(rr),
			unsafe.Sizeof(rr.PlayerBJ) + unsafe.Sizeof(rr.MainWagered) + unsafe.Sizeof(rr.DealerBJ) +
				unsafe.Sizeof(rr.Action) + unsafe.Sizeof(rr.DealerPlayed) + unsafe.Sizeof(rr.MainNet) +
				unsafe.Sizeof(rr.DealerBust) + unsafe.Sizeof(rr.SideWagered) + unsafe.Sizeof(rr.Hands) +
				unsafe.Sizeof(rr.SideNet) + unsafe.Sizeof(rr.Log)},
		{"Shoe", unsafe.Sizeof(sh), 0},
	}

	const cacheLine = 64
	t.Logf("%-14s %6s %8s %10s %s", "structure", "taille", "champs", "remplissage", "par ligne de cache 64 o")
	for _, r := range rows {
		pad := "—"
		if r.fields > 0 {
			pad = itoa(int(r.size-r.fields)) + " o"
		}
		per := "—"
		if r.size > 0 && r.size <= cacheLine {
			per = itoa(cacheLine / int(r.size))
		}
		t.Logf("%-14s %4d o %6d o %10s %s", r.name, r.size, r.fields, pad, per)
	}

	// Une carte compacte tiendrait dans un octet : rang sur 4 bits, enseigne
	// sur 2 bits. Le rapport entre les deux est l'enjeu du premier palier.
	t.Logf("")
	t.Logf("Une carte compacte occuperait 1 octet, soit un facteur %d sur la représentation,", unsafe.Sizeof(c))
	t.Logf("et %d cartes par ligne de cache au lieu de %d.", cacheLine, cacheLine/int(unsafe.Sizeof(c)))
	t.Logf("Les cartes étant de plus manipulées par pointeur, chacune est allouée")
	t.Logf("séparément et aucune garantie de localité ne subsiste.")
}
