package blackjack

import (
	"math"
	"testing"
)

// TestTableAccounting vérifie que la comptabilité de la table est exacte à
// chaque coup : la variation du solde doit égaler le résultat net annoncé,
// mises annexes, doubles et séparations comprises.
//
// C'est l'invariant qui empêche une interface de « créer » ou de « perdre »
// de l'argent silencieusement.
func TestTableAccounting(t *testing.T) {
	r := DefaultRules()
	// Solde volontairement énorme : on ne veut jamais être empêché de doubler
	// ou de séparer, pour exercer tous les chemins.
	tb := NewTable(r, 1e9, 4242)

	side := SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}

	for i := 0; i < 20_000; i++ {
		before := tb.Bankroll
		if err := tb.Deal(10, side); err != nil {
			t.Fatalf("coup %d : %v", i, err)
		}
		if err := playOutWithHint(t, tb, i); err != nil {
			t.Fatal(err)
		}
		got := tb.Bankroll - before
		if math.Abs(got-tb.roundNet) > 1e-6 {
			t.Fatalf("coup %d : solde varié de %.4f mais net annoncé %.4f",
				i, got, tb.roundNet)
		}
	}
}

// TestTableCrossValidation est la vérification croisée du projet.
//
// La table interactive (Table) et la boucle de simulation (PlayRound) sont
// deux implémentations distinctes du même règlement : la première pilote le
// coup pas à pas pour un humain, la seconde le déroule d'un bloc et c'est elle
// qui est mesurée. Si elles divergent, l'une des deux est fausse.
//
// En faisant jouer la table par la stratégie de base — exactement ce
// qu'applique la simulation — les deux doivent produire le même avantage de la
// maison aux fluctuations statistiques près.
func TestTableCrossValidation(t *testing.T) {
	const rounds = 300_000
	r := DefaultRules()

	tb := NewTable(r, 1e9, 2026)
	var net float64

	splits, doubles, surrenders, insurances := 0, 0, 0, 0

	for i := 0; i < rounds; i++ {
		if err := tb.Deal(1, SideBets{}); err != nil {
			t.Fatalf("coup %d : %v", i, err)
		}
		if tb.phase == PhaseInsurance {
			insurances++
		}
		if err := playOutWithHint(t, tb, i); err != nil {
			t.Fatal(err)
		}
		if len(tb.hands) > 1 {
			splits++
		}
		for _, ph := range tb.hands {
			if ph.h.Doubled {
				doubles++
			}
			if ph.h.Surrendered {
				surrenders++
			}
		}
		net += tb.roundNet
	}

	tableEdge := -net / float64(rounds) * 100

	// Même règlement, même stratégie, par l'autre chemin de code.
	st := Simulate(rounds, 2026, r, 1, SideBets{})
	simEdge := st.HouseEdge() * 100
	stderr := st.StdError() * 100

	t.Logf("avantage via Table     : %+.4f %%", tableEdge)
	t.Logf("avantage via PlayRound : %+.4f %% (erreur-type %.4f)", simEdge, stderr)
	t.Logf("écart                  : %.4f point", math.Abs(tableEdge-simEdge))
	t.Logf("chemins exercés        : %d séparations, %d doubles, %d abandons, %d assurances proposées",
		splits, doubles, surrenders, insurances)

	// Les deux chemins tirent des cartes différentes (les décisions humaines
	// n'épuisent pas le sabot au même rythme), donc l'écart admis est celui de
	// deux mesures indépendantes : la somme quadratique de leurs erreurs-types,
	// élargie à 4 sigma.
	tol := 4 * stderr * math.Sqrt2
	if math.Abs(tableEdge-simEdge) > tol {
		t.Fatalf("les deux chemins divergent de %.4f point, tolérance %.4f : l'un des deux règlements est faux",
			math.Abs(tableEdge-simEdge), tol)
	}

	// Sans ces chemins exercés, la validation croisée ne prouverait rien.
	if splits == 0 || doubles == 0 || surrenders == 0 || insurances == 0 {
		t.Errorf("certains chemins de jeu n'ont jamais été exercés")
	}
}

// TestTableHideHoleCard vérifie que la carte cachée du croupier ne fuite
// jamais vers l'interface avant son dévoilement.
func TestTableHideHoleCard(t *testing.T) {
	r := DefaultRules()
	r.OfferInsurance = false
	tb := NewTable(r, 1000, 11)

	for i := 0; i < 500; i++ {
		if err := tb.Deal(1, SideBets{}); err != nil {
			t.Fatal(err)
		}
		v := tb.View()
		if v.Phase == PhasePlaying {
			if len(v.Dealer.Cards) != 1 || !v.Dealer.Hidden {
				t.Fatalf("coup %d : la carte cachée a fuité (%d cartes visibles, hidden=%v)",
					i, len(v.Dealer.Cards), v.Dealer.Hidden)
			}
			// Le total annoncé ne doit refléter que la carte visible.
			visible := &Hand{Cards: []*Card{tb.dealer.Cards[0]}}
			want, _ := visible.Total()
			if v.Dealer.Total != want {
				t.Fatalf("coup %d : total du croupier %d annoncé alors que sa carte visible vaut %d",
					i, v.Dealer.Total, want)
			}
		}
		if err := playOutWithHint(t, tb, i); err != nil {
			t.Fatal(err)
		}
		if v := tb.View(); v.Dealer.Hidden {
			t.Fatalf("coup %d : la carte cachée n'a pas été dévoilée au règlement", i)
		}
	}
}

// TestTableLegalActionsOnly vérifie qu'aucune action hors de la liste légale
// n'est acceptée.
func TestTableLegalActionsOnly(t *testing.T) {
	tb := NewTable(DefaultRules(), 1000, 5)
	if err := tb.Deal(10, SideBets{}); err != nil {
		t.Fatal(err)
	}
	if tb.phase == PhaseInsurance {
		if err := tb.Act(ActionDecline); err != nil {
			t.Fatal(err)
		}
	}
	if tb.phase != PhasePlaying {
		t.Skip("coup résolu immédiatement, cas non pertinent pour ce test")
	}
	if err := tb.Act("tricher"); err == nil {
		t.Fatal("une action inconnue a été acceptée")
	}
	for _, a := range []string{ActionInsure, ActionDecline} {
		if err := tb.Act(a); err == nil {
			t.Fatalf("l'action %q a été acceptée hors de la phase d'assurance", a)
		}
	}
}

// playOutWithHint joue le coup en cours jusqu'à son terme en suivant la
// stratégie de base, et vérifie au passage que le conseil donné figure
// toujours parmi les actions autorisées.
func playOutWithHint(t *testing.T, tb *Table, round int) error {
	t.Helper()
	for guard := 0; tb.phase == PhaseInsurance || tb.phase == PhasePlaying; guard++ {
		if guard > 64 {
			t.Fatalf("coup %d : le coup ne se termine pas", round)
		}
		a := tb.Hint()
		if a == "" {
			t.Fatalf("coup %d : aucun conseil disponible en phase %s", round, tb.phase)
		}
		if !contains(tb.LegalActions(), a) {
			t.Fatalf("coup %d : la stratégie conseille %q, absent des actions autorisées %v",
				round, a, tb.LegalActions())
		}
		if err := tb.Act(a); err != nil {
			return err
		}
	}
	if tb.phase != PhaseDone {
		t.Fatalf("coup %d : phase finale inattendue %s", round, tb.phase)
	}
	for i, ph := range tb.hands {
		if ph.result == "" {
			t.Fatalf("coup %d : la main %d n'a pas de résultat", round, i)
		}
	}
	return nil
}
