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

// Puits de benchmark.
//
// MODULE 4, piège de l'élimination de code mort (DCE) : si le résultat d'un
// calcul n'est jamais lu hors de la boucle, le compilateur supprime purement et
// simplement ce calcul du binaire. Le benchmark mesure alors une boucle vide.
//
// Le risque est d'autant plus réel que la fonction mesurée est petite et
// inlinable — ce qui est devenu le cas de Hand.Total et Card.Value après le
// rang 2. Chaque benchmark accumule donc son résultat dans une variable locale,
// affectée à l'une de ces variables de paquet après la boucle : le compilateur
// ne peut plus prouver que le calcul est inutile.
var (
	sinkInt     int
	sinkBool    bool
	sinkFloat   float64
	sinkCard    Card
	sinkString  string
	sinkOutcome SideOutcome
	sinkStats   Stats
)

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

	var net float64
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.CutReached() {
			s.Shuffle()
		}
		net += PlayRound(s, r, 1, sb, nil).MainNet
	}
	sinkFloat = net
}

// BenchmarkPlayRoundSideBets mesure le surcoût des quatre paris annexes, qui
// imposent de suivre les enseignes et d'évaluer des combinaisons de poker.
func BenchmarkPlayRoundSideBets(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)
	sb := SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}

	var net float64
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.CutReached() {
			s.Shuffle()
		}
		net += PlayRound(s, r, 1, sb, nil).MainNet
	}
	sinkFloat = net
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
	// Shuffle n'a pas de valeur de retour : on lit l'état qu'elle a produit.
	sinkInt = s.Remaining()
}

// BenchmarkDeal isole la distribution d'une carte.
func BenchmarkDeal(b *testing.B) {
	r := DefaultRules()
	s := benchShoe(r, 42)

	var c Card
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.Remaining() == 0 {
			s.Shuffle()
		}
		c = s.Deal()
	}
	sinkCard = c
}

// BenchmarkHandTotal isole le calcul du total d'une main. La fonction est
// appelée plusieurs fois par décision, ce qui en fait la plus chaude du
// moteur.
func BenchmarkHandTotal(b *testing.B) {
	h := &Hand{Cards: []Card{
		newCard(rankAce, suitPique),
		newCard(rank7, suitCoeur),
		newCard(rank5, suitCarreau),
	}}

	var total int
	var soft bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t, s := h.Total()
		total += t
		soft = s
	}
	sinkInt, sinkBool = total, soft
}

// BenchmarkDecide isole la consultation de la stratégie de base : construction
// d'une clé textuelle puis hachage.
//
// Mesure l'appel tel que le moteur le fait, c'est-à-dire direct. Mesurer
// DefaultStrategy.Decide() au travers de l'interface mesurerait un chemin que
// le moteur n'emprunte plus.
func BenchmarkDecide(b *testing.B) {
	r := DefaultRules()
	h := &Hand{Cards: []Card{
		newCard(rank10, suitPique),
		newCard(rank6, suitCoeur),
	}}
	up := newCard(rank9, suitTrefle)

	var d string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d = Decide(h, up, r, 1)
	}
	sinkString = d
}

// BenchmarkSideBetEval isole l'évaluation des paris annexes, dont le 21+3 qui
// trie trois cartes pour détecter une suite.
func BenchmarkSideBetEval(b *testing.B) {
	a := newCard(rank5, suitPique)
	c2 := newCard(rank6, suitPique)
	up := newCard(rank7, suitPique)

	var o SideOutcome
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o += EvalPerfectPairs(a, c2)
		o += EvalTwentyOnePlus3(a, c2, up)
		o += EvalLuckyLadies(a, c2, false)
	}
	sinkOutcome = o
}

// BenchmarkSimulate mesure la boucle complète sur un nombre fixe de coups.
// C'est l'équivalent interne de la mesure de bout en bout faite par hyperfine
// sur le binaire.
func BenchmarkSimulate(b *testing.B) {
	r := DefaultRules()
	var st Stats
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st = Simulate(10_000, 42, r, 1, SideBets{})
	}
	sinkStats = st
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
		{"Card", unsafe.Sizeof(c), unsafe.Sizeof(c)},
		{"Hand", unsafe.Sizeof(h),
			unsafe.Sizeof(h.Doubled) + unsafe.Sizeof(h.Bet) + unsafe.Sizeof(h.FromSplit) +
				unsafe.Sizeof(h.Cards) + unsafe.Sizeof(h.SplitAce) + unsafe.Sizeof(h.Stood) +
				unsafe.Sizeof(h.Surrendered)},
		{"RoundResult", unsafe.Sizeof(rr),
			unsafe.Sizeof(rr.PlayerBJ) + unsafe.Sizeof(rr.MainWagered) + unsafe.Sizeof(rr.DealerBJ) +
				unsafe.Sizeof(rr.Action) + unsafe.Sizeof(rr.DealerPlayed) + unsafe.Sizeof(rr.MainNet) +
				unsafe.Sizeof(rr.DealerBust) + unsafe.Sizeof(rr.SideWagered) + unsafe.Sizeof(rr.Hands) +
				unsafe.Sizeof(rr.SideNet)},
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

	// Depuis le rang 2 du profil, une carte occupe un octet et se stocke par
	// valeur. La version de référence en utilisait 32, par pointeur.
	t.Logf("")
	t.Logf("Card occupe %d octet(s), contre 32 dans la version de référence : facteur %d.",
		unsafe.Sizeof(c), 32/int(unsafe.Sizeof(c)))
	t.Logf("Une ligne de cache de %d octets contient donc %d cartes, contre 2 avant.",
		cacheLine, cacheLine/int(unsafe.Sizeof(c)))
	t.Logf("Un sabot de 4 jeux occupe %d octets contigus, soit %d lignes de cache,",
		208*int(unsafe.Sizeof(c)), 208*int(unsafe.Sizeof(c))/cacheLine)
	t.Logf("contre 6 656 octets sur 104 lignes et 208 objets disperses dans le tas.")
}
