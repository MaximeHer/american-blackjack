package blackjack

import "errors"

// Erreurs renvoyées par Table.
var (
	errBadPhase          = errors.New("action impossible dans cette phase du coup")
	errBadBet            = errors.New("la mise doit être positive")
	errBadAction         = errors.New("action non autorisée")
	errInsufficientFunds = errors.New("solde insuffisant")
)

// Les types ci-dessous ne servent qu'à sérialiser l'état de la table vers
// l'interface. Ils sont volontairement séparés des structures de jeu : aucun
// champ d'affichage ne doit alourdir Card, Hand ou Shoe, qui sont les
// structures mesurées.

// CardView est une carte telle qu'affichée. La carte cachée du croupier n'est
// jamais sérialisée avant son dévoilement.
type CardView struct {
	Rank string `json:"rank"`
	Suit string `json:"suit"`
}

// HandView est une main du joueur telle qu'affichée.
type HandView struct {
	Cards       []CardView `json:"cards"`
	Total       int        `json:"total"`
	Soft        bool       `json:"soft"`
	Bet         float64    `json:"bet"`
	Bust        bool       `json:"bust"`
	Blackjack   bool       `json:"blackjack"`
	Doubled     bool       `json:"doubled"`
	Surrendered bool       `json:"surrendered"`
	FromSplit   bool       `json:"fromSplit"`
	Active      bool       `json:"active"`
	Done        bool       `json:"done"`
	Result      string     `json:"result"`
	Payout      float64    `json:"payout"`
}

// DealerView est la main du croupier telle qu'affichée.
type DealerView struct {
	Cards     []CardView `json:"cards"`
	Hidden    bool       `json:"hidden"`
	Total     int        `json:"total"`
	Soft      bool       `json:"soft"`
	Bust      bool       `json:"bust"`
	Blackjack bool       `json:"blackjack"`
}

// ShoeView décrit l'état du sabot.
type ShoeView struct {
	Remaining  int  `json:"remaining"`
	Size       int  `json:"size"`
	Shuffles   int  `json:"shuffles"`
	CutReached bool `json:"cutReached"`
}

// RulesView expose les règles de la table à l'interface.
type RulesView struct {
	NumDecks         int     `json:"numDecks"`
	Penetration      float64 `json:"penetration"`
	DealerHitsSoft17 bool    `json:"dealerHitsSoft17"`
	BlackjackPayout  float64 `json:"blackjackPayout"`
	DoubleAfterSplit bool    `json:"doubleAfterSplit"`
	MaxSplitHands    int     `json:"maxSplitHands"`
	LateSurrender    bool    `json:"lateSurrender"`
}

// TableView est l'état complet et sérialisable de la table.
type TableView struct {
	Phase        Phase        `json:"phase"`
	Bankroll     float64      `json:"bankroll"`
	Bet          float64      `json:"bet"`
	SideBets     SideBets     `json:"sideBets"`
	Dealer       DealerView   `json:"dealer"`
	Hands        []HandView   `json:"hands"`
	ActiveHand   int          `json:"activeHand"`
	LegalActions []string     `json:"legalActions"`
	Hint         string       `json:"hint"`
	SideResults  []SideResult `json:"sideResults"`
	Messages     []string     `json:"messages"`
	Shoe         ShoeView     `json:"shoe"`
	RoundNet     float64      `json:"roundNet"`
	Rules        RulesView    `json:"rules"`
}

// View sérialise l'état courant de la table.
func (t *Table) View() TableView {
	v := TableView{
		Phase:        t.phase,
		Bankroll:     t.Bankroll,
		Bet:          t.bet,
		SideBets:     t.side,
		ActiveHand:   t.active,
		LegalActions: t.LegalActions(),
		Hint:         t.Hint(),
		SideResults:  t.sideResults,
		Messages:     t.messages,
		RoundNet:     t.roundNet,
		Shoe: ShoeView{
			Remaining:  t.shoe.Remaining(),
			Size:       t.shoe.Size(),
			Shuffles:   t.shoe.Shuffles,
			CutReached: t.shoe.CutReached(),
		},
		Rules: RulesView{
			NumDecks:         t.rules.NumDecks,
			Penetration:      t.rules.Penetration,
			DealerHitsSoft17: t.rules.DealerHitsSoft17,
			BlackjackPayout:  t.rules.BlackjackPayout,
			DoubleAfterSplit: t.rules.DoubleAfterSplit,
			MaxSplitHands:    t.rules.MaxSplitHands,
			LateSurrender:    t.rules.LateSurrender,
		},
	}

	if t.dealer != nil {
		v.Dealer = t.dealerView()
	}
	for i, ph := range t.hands {
		v.Hands = append(v.Hands, HandView{
			Cards:       cardViews(ph.h.Cards),
			Total:       totalOf(ph.h),
			Soft:        softOf(ph.h),
			Bet:         ph.h.Bet,
			Bust:        ph.h.IsBust(),
			Blackjack:   ph.h.IsBlackjack(),
			Doubled:     ph.h.Doubled,
			Surrendered: ph.h.Surrendered,
			FromSplit:   ph.h.FromSplit,
			Active:      t.phase == PhasePlaying && i == t.active,
			Done:        ph.done,
			Result:      ph.result,
			Payout:      ph.payout,
		})
	}
	return v
}

// dealerView masque la carte cachée tant qu'elle n'est pas dévoilée, et ne
// révèle alors qu'un total partiel. L'interface ne doit jamais recevoir une
// information que le joueur n'a pas le droit de connaître.
func (t *Table) dealerView() DealerView {
	if !t.revealed {
		visible := &Hand{Cards: []Card{t.dealer.Cards[0]}}
		total, soft := visible.Total()
		return DealerView{
			Cards:  cardViews(visible.Cards),
			Hidden: true,
			Total:  total,
			Soft:   soft,
		}
	}
	total, soft := t.dealer.Total()
	return DealerView{
		Cards:     cardViews(t.dealer.Cards),
		Hidden:    false,
		Total:     total,
		Soft:      soft,
		Bust:      t.dealer.IsBust(),
		Blackjack: t.dealer.IsBlackjack(),
	}
}

func cardViews(cards []Card) []CardView {
	out := make([]CardView, 0, len(cards))
	for _, c := range cards {
		out = append(out, CardView{Rank: c.Rank, Suit: c.Suit})
	}
	return out
}

func totalOf(h *Hand) int { t, _ := h.Total(); return t }
func softOf(h *Hand) bool { _, s := h.Total(); return s }
