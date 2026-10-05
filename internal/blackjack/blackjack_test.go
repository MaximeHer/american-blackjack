package blackjack

import (
	"math"
	"math/rand"
	"testing"
)

func c(rank, suit string) *Card { return &Card{Rank: rank, Suit: suit} }

func TestHandTotal(t *testing.T) {
	cases := []struct {
		name  string
		cards []*Card
		total int
		soft  bool
	}{
		{"blackjack", []*Card{c("A", "Pique"), c("K", "Coeur")}, 21, true},
		{"as souple", []*Card{c("A", "Pique"), c("6", "Coeur")}, 17, true},
		{"as durci", []*Card{c("A", "Pique"), c("6", "Coeur"), c("9", "Trefle")}, 16, false},
		{"deux as", []*Card{c("A", "Pique"), c("A", "Coeur")}, 12, true},
		{"trois as", []*Card{c("A", "Pique"), c("A", "Coeur"), c("A", "Trefle")}, 13, true},
		{"saute", []*Card{c("K", "Pique"), c("Q", "Coeur"), c("5", "Trefle")}, 25, false},
		{"dur 20", []*Card{c("K", "Pique"), c("Q", "Coeur")}, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Hand{Cards: tc.cards}
			total, soft := h.Total()
			if total != tc.total || soft != tc.soft {
				t.Fatalf("Total() = (%d, %v), attendu (%d, %v)", total, soft, tc.total, tc.soft)
			}
		})
	}
}

func TestBlackjackNotAfterSplit(t *testing.T) {
	h := &Hand{Cards: []*Card{c("A", "Pique"), c("K", "Coeur")}, FromSplit: true}
	if h.IsBlackjack() {
		t.Fatal("un 21 issu d'un split ne doit pas être un blackjack")
	}
}

func TestIsPairAcrossTenValues(t *testing.T) {
	h := &Hand{Cards: []*Card{c("K", "Pique"), c("Q", "Coeur")}}
	if !h.IsPair() {
		t.Fatal("Roi et Dame valent tous deux 10 et forment une paire séparable")
	}
}

func TestPerfectPairs(t *testing.T) {
	cases := []struct {
		name  string
		a, b  *Card
		mult  float64
		label string
	}{
		{"parfaite", c("8", "Coeur"), c("8", "Coeur"), 25, "parfaite"},
		{"coloree", c("8", "Coeur"), c("8", "Carreau"), 12, "coloree"},
		{"mixte", c("8", "Coeur"), c("8", "Pique"), 6, "mixte"},
		{"perdu", c("8", "Coeur"), c("9", "Coeur"), 0, "perdu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, l := EvalPerfectPairs(tc.a, tc.b)
			if m != tc.mult || l != tc.label {
				t.Fatalf("= (%v, %q), attendu (%v, %q)", m, l, tc.mult, tc.label)
			}
		})
	}
}

func TestTwentyOnePlus3(t *testing.T) {
	cases := []struct {
		name     string
		a, b, up *Card
		label    string
	}{
		{"brelan couleur", c("7", "Coeur"), c("7", "Coeur"), c("7", "Coeur"), "brelan_couleur"},
		{"quinte flush", c("5", "Pique"), c("6", "Pique"), c("7", "Pique"), "quinte_flush"},
		{"brelan", c("7", "Coeur"), c("7", "Pique"), c("7", "Trefle"), "brelan"},
		{"quinte", c("5", "Coeur"), c("6", "Pique"), c("7", "Trefle"), "quinte"},
		{"quinte haute", c("Q", "Coeur"), c("K", "Pique"), c("A", "Trefle"), "quinte"},
		{"quinte basse", c("A", "Coeur"), c("2", "Pique"), c("3", "Trefle"), "quinte"},
		{"couleur", c("2", "Coeur"), c("7", "Coeur"), c("K", "Coeur"), "couleur"},
		{"perdu", c("2", "Coeur"), c("7", "Pique"), c("K", "Trefle"), "perdu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, l := EvalTwentyOnePlus3(tc.a, tc.b, tc.up); l != tc.label {
				t.Fatalf("= %q, attendu %q", l, tc.label)
			}
		})
	}
}

func TestLuckyLadies(t *testing.T) {
	if _, l := EvalLuckyLadies(c("Q", "Coeur"), c("Q", "Coeur"), true); l != "paire_dame_coeur_bj" {
		t.Fatalf("paire de Dames de coeur avec blackjack croupier = %q", l)
	}
	if _, l := EvalLuckyLadies(c("Q", "Coeur"), c("Q", "Coeur"), false); l != "paire_dame_coeur" {
		t.Fatalf("paire de Dames de coeur = %q", l)
	}
	if _, l := EvalLuckyLadies(c("K", "Pique"), c("K", "Pique"), false); l != "vingt_identique" {
		t.Fatalf("deux Rois de pique = %q", l)
	}
	if _, l := EvalLuckyLadies(c("K", "Pique"), c("Q", "Pique"), false); l != "vingt_couleur" {
		t.Fatalf("20 de même enseigne = %q", l)
	}
	if _, l := EvalLuckyLadies(c("K", "Pique"), c("Q", "Coeur"), false); l != "vingt" {
		t.Fatalf("20 quelconque = %q", l)
	}
	if _, l := EvalLuckyLadies(c("K", "Pique"), c("9", "Coeur"), false); l != "perdu" {
		t.Fatalf("19 ne doit rien payer, obtenu %q", l)
	}
}

func TestBuster(t *testing.T) {
	if m, _ := EvalBuster(5, false); m != 0 {
		t.Fatal("sans dépassement du croupier, le Buster est perdu")
	}
	if m, _ := EvalBuster(6, true); m != 18 {
		t.Fatalf("un dépassement en 6 cartes paie 18:1, obtenu %v", m)
	}
	if m, _ := EvalBuster(12, true); m != 250 {
		t.Fatalf("au-delà de 8 cartes, le palier maximal s'applique, obtenu %v", m)
	}
}

func TestShoeComposition(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	s := NewShoe(4, 0.75, rng)
	if got := s.Remaining(); got != 208 {
		t.Fatalf("un sabot de 4 jeux compte 208 cartes, obtenu %d", got)
	}

	counts := map[string]int{}
	for s.Remaining() > 0 {
		card := s.Deal()
		counts[card.Rank+card.Suit]++
	}
	if len(counts) != 52 {
		t.Fatalf("52 cartes distinctes attendues, obtenu %d", len(counts))
	}
	for k, n := range counts {
		if n != 4 {
			t.Fatalf("la carte %s apparaît %d fois au lieu de 4", k, n)
		}
	}
}

func TestDeterminisme(t *testing.T) {
	r := DefaultRules()
	sb := SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}
	a := Simulate(20_000, 42, r, 1, sb)
	b := Simulate(20_000, 42, r, 1, sb)
	if a != b {
		t.Fatal("à graine égale, deux simulations doivent être rigoureusement identiques")
	}
}

// expectedHouseEdge est l'avantage de la maison publié pour les règles de
// DefaultRules : 4 jeux, croupier restant sur 17 souple, blackjack payé 3:2,
// double après split et abandon tardif autorisés, resplit jusqu'à 4 mains.
const expectedHouseEdge = 0.35 // en pourcentage de la mise initiale

// TestHouseEdgeOracle est le test de non-régression central du projet.
//
// Il ne vérifie pas une valeur arbitraire mais une grandeur publiée, et il
// borne l'écart accepté par l'erreur-type réellement mesurée plutôt que par
// une marge choisie à la main. L'écart-type du résultat par coup valant
// environ 1,14 unité de mise, deux millions de coups ne résolvent l'avantage
// de la maison qu'à environ 0,08 point près : la tolérance doit en tenir
// compte, sinon le test échoue au hasard.
//
// Toute optimisation ultérieure doit laisser ce chiffre inchangé. S'il dérive
// au-delà du bruit statistique, la logique de jeu a été cassée.
func TestHouseEdgeOracle(t *testing.T) {
	st := Simulate(2_000_000, 42, DefaultRules(), 1, SideBets{})

	edge := st.HouseEdge() * 100
	stderr := st.StdError() * 100
	ecart := math.Abs(edge - expectedHouseEdge)

	t.Logf("avantage de la maison : %+.4f %% (erreur-type %.4f point)", edge, stderr)
	t.Logf("écart au publié       : %.4f point, soit %.2f erreurs-types", ecart, ecart/stderr)
	t.Logf("element of risk       : %+.4f %%", st.ElementOfRisk()*100)
	t.Logf("écart-type par coup   : %.4f unité de mise", st.StdDev())
	t.Logf("mains par coup        : %.4f", float64(st.Hands)/float64(st.Rounds))

	// Quatre erreurs-types : la probabilité d'un échec fortuit est négligeable,
	// mais une vraie régression de logique sort largement de cette borne.
	if ecart > 4*stderr {
		t.Fatalf("avantage de la maison à %.2f erreurs-types du publié (%+.4f %% contre %.2f %%)",
			ecart/stderr, edge, expectedHouseEdge)
	}
}

// TestFrequencesConnues valide indirectement la distribution et la logique de
// jeu à partir de grandeurs connues du blackjack.
func TestFrequencesConnues(t *testing.T) {
	st := Simulate(2_000_000, 7, DefaultRules(), 1, SideBets{})

	bjRate := float64(st.PlayerBJ) / float64(st.Rounds) * 100

	t.Logf("blackjacks joueur    : %.3f %% (attendu ~4,75 %%)", bjRate)

	// Probabilité d'un blackjack servi sur deux cartes.
	if math.Abs(bjRate-4.75) > 0.3 {
		t.Errorf("taux de blackjack joueur suspect : %.3f %%, attendu ~4,75 %%", bjRate)
	}

	// Le taux de dépassement du croupier observé en cours de partie n'est
	// volontairement pas comparé ici à la valeur publiée de 28 %, parce qu'il
	// est biaisé par construction : le croupier ne complète pas sa main quand
	// toutes les mains du joueur ont sauté, et ces coups-là sont justement
	// ceux où il montrait une carte forte, qui saute peu. L'échantillon des
	// mains effectivement jouées est donc enrichi en cartes faibles, qui
	// sautent beaucoup, et le taux conditionnel dépasse logiquement 28 %.
	//
	// La valeur inconditionnelle est validée séparément, indépendamment du
	// joueur, par TestDealerBustRateInconditionnel.
	t.Logf("croupier sauté       : %.3f %% des mains jouées (taux conditionnel, biaisé)",
		float64(st.DealerBust)/float64(st.DealerPlayed)*100)
	t.Logf("croupier a joué      : %.2f %% des coups",
		float64(st.DealerPlayed)/float64(st.Rounds)*100)
}

// TestDealerBustRateInconditionnel valide playDealer sur la seule grandeur
// qui se compare proprement à la littérature : la probabilité qu'un croupier
// saute, mesurée sur des mains tirées du sabot et systématiquement jouées
// jusqu'au bout, sans aucun conditionnement par le jeu du joueur.
//
// Pour un croupier restant sur 17 souple, cette probabilité vaut environ
// 28,3 %, blackjacks du croupier compris.
func TestDealerBustRateInconditionnel(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	s := NewShoe(4, 0.75, rng)
	r := DefaultRules()

	const n = 2_000_000
	bust := 0
	for i := 0; i < n; i++ {
		if s.CutReached() {
			s.Shuffle()
		}
		d := &Hand{Cards: []*Card{s.Deal(), s.Deal()}}
		playDealer(d, s, r, nil)
		if d.IsBust() {
			bust++
		}
	}

	rate := float64(bust) / float64(n) * 100
	t.Logf("dépassement du croupier, toutes mains jouées : %.3f %% (attendu ~28,3 %%)", rate)

	if math.Abs(rate-28.3) > 1 {
		t.Errorf("taux de dépassement inconditionnel suspect : %.3f %%, attendu ~28,3 %%", rate)
	}
}
