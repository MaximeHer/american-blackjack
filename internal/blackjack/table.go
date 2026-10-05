package blackjack

import "math/rand"

// Table est une table de blackjack jouable coup par coup, par un humain.
//
// Ce type est DÉLIBÉRÉMENT SÉPARÉ de PlayRound. La boucle de simulation est
// l'objet des mesures de performance : elle doit rester dans son état de
// référence, sans instrumentation, sans indirection et sans champ ajouté pour
// les besoins de l'affichage. Table réutilise les mêmes règles, le même sabot
// et les mêmes évaluations de paris annexes, mais pilote le coup pas à pas au
// lieu de le dérouler d'un bloc.
//
// Pour la même raison, l'état propre au jeu interactif (une main a-t-elle déjà
// agi, quel est son résultat) vit dans playHand et non dans Hand : grossir
// Hand dégraderait la structure mesurée.
type Table struct {
	rules Rules
	shoe  *Shoe

	Bankroll float64

	phase    Phase
	bet      float64
	side     SideBets
	hands    []*playHand
	dealer   *Hand
	active   int
	revealed bool

	insuranceOffered bool
	insuranceTaken   bool

	sideResults []SideResult
	roundNet    float64
	messages    []string
}

// Phase décrit où en est le coup.
type Phase string

const (
	PhaseBetting   Phase = "betting"   // en attente d'une mise
	PhaseInsurance Phase = "insurance" // assurance proposée
	PhasePlaying   Phase = "playing"   // le joueur décide
	PhaseDone      Phase = "done"      // coup réglé
)

// Actions acceptées par Act.
const (
	ActionHit       = "hit"
	ActionStand     = "stand"
	ActionDouble    = "double"
	ActionSplit     = "split"
	ActionSurrender = "surrender"
	ActionInsure    = "insure"
	ActionDecline   = "decline"
)

// playHand enveloppe une main du joueur avec l'état propre au jeu interactif.
type playHand struct {
	h      *Hand
	acted  bool
	done   bool
	result string
	payout float64
}

// SideResult rapporte le dénouement d'un pari annexe.
type SideResult struct {
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Stake float64 `json:"stake"`
	Net   float64 `json:"net"`
}

// NewTable ouvre une table. Si seed est nul, le sabot est mélangé de façon
// imprévisible ; sinon la partie est reproductible.
func NewTable(r Rules, bankroll float64, seed int64) *Table {
	rng := rand.New(rand.NewSource(seed))
	return &Table{
		rules:    r,
		shoe:     NewShoe(r.NumDecks, r.Penetration, rng),
		Bankroll: bankroll,
		phase:    PhaseBetting,
	}
}

// Rules expose les règles de la table.
func (t *Table) Rules() Rules { return t.rules }

// Deal engage les mises et distribue la main initiale.
func (t *Table) Deal(bet float64, side SideBets) error {
	if t.phase != PhaseBetting && t.phase != PhaseDone {
		return errBadPhase
	}
	if bet <= 0 {
		return errBadBet
	}
	stake := bet + side.Total()
	if stake > t.Bankroll {
		return errInsufficientFunds
	}

	// Le sabot se rebat entre deux coups, jamais au milieu d'un coup.
	if t.shoe.CutReached() {
		t.shoe.Shuffle()
	}

	t.Bankroll -= stake
	t.bet = bet
	t.side = side
	t.revealed = false
	t.insuranceOffered = false
	t.insuranceTaken = false
	t.sideResults = nil
	t.roundNet = 0
	t.messages = nil
	t.active = 0

	p1 := t.shoe.Deal()
	up := t.shoe.Deal()
	p2 := t.shoe.Deal()
	hole := t.shoe.Deal()

	t.dealer = &Hand{Cards: []*Card{up, hole}}
	t.hands = []*playHand{{h: &Hand{Cards: []*Card{p1, p2}, Bet: bet}}}

	// Paris annexes jugés sur la seule distribution initiale. Lucky Ladies
	// attend le contrôle de la carte cachée, dont dépend son gain maximal.
	if side.PerfectPairs > 0 {
		m, label := EvalPerfectPairs(p1, p2)
		t.addSide("Perfect Pairs", label, side.PerfectPairs, m)
	}
	if side.TwentyOnePlus3 > 0 {
		m, label := EvalTwentyOnePlus3(p1, p2, up)
		t.addSide("21+3", label, side.TwentyOnePlus3, m)
	}

	// L'assurance se propose avant tout autre choix, sur un As visible.
	if t.rules.OfferInsurance && up.IsAce() && t.Bankroll >= bet/2 {
		t.insuranceOffered = true
		t.phase = PhaseInsurance
		return nil
	}

	t.peek()
	return nil
}

// Act applique une action du joueur.
func (t *Table) Act(action string) error {
	switch t.phase {
	case PhaseInsurance:
		switch action {
		case ActionInsure:
			t.Bankroll -= t.bet / 2
			t.insuranceTaken = true
			t.peek()
			return nil
		case ActionDecline:
			t.peek()
			return nil
		}
		return errBadAction

	case PhasePlaying:
		return t.playerAct(action)
	}
	return errBadPhase
}

// peek fait contrôler la carte cachée au croupier quand sa carte visible le
// justifie, puis engage la phase de jeu ou règle immédiatement le coup.
func (t *Table) peek() {
	up := t.dealer.Cards[0]
	playerBJ := t.hands[0].h.IsBlackjack()

	if up.IsAce() || up.IsTenValue() {
		if t.dealer.IsBlackjack() {
			t.revealed = true
			t.resolveLuckyLadies(true)

			if t.insuranceTaken {
				// L'assurance paie 2:1 : la demi-mise est rendue, majorée du
				// double.
				t.Bankroll += t.bet * 1.5
				t.roundNet += t.bet
				t.note("Assurance gagnée, payée 2:1.")
			}

			if playerBJ {
				t.Bankroll += t.bet
				t.settleHand(t.hands[0], "egalite", t.bet)
				t.note("Blackjack des deux côtés : égalité.")
			} else {
				t.settleHand(t.hands[0], "perdu", 0)
				t.note("Blackjack du croupier.")
			}

			t.resolveBusterNoDraw()
			t.phase = PhaseDone
			return
		}
	}

	if t.insuranceTaken {
		t.roundNet -= t.bet / 2
		t.note("Assurance perdue.")
	}
	t.resolveLuckyLadies(false)

	if playerBJ {
		t.revealed = true
		ret := t.bet * (1 + t.rules.BlackjackPayout)
		t.Bankroll += ret
		t.settleHand(t.hands[0], "blackjack", ret)
		t.note("Blackjack ! Payé 3:2.")
		if t.side.Buster > 0 {
			playDealer(t.dealer, t.shoe, t.rules, nil)
			t.resolveBuster()
		} else {
			t.resolveBusterNoDraw()
		}
		t.phase = PhaseDone
		return
	}

	t.phase = PhasePlaying
	t.autoAdvance()
}

// playerAct applique une action sur la main active.
func (t *Table) playerAct(action string) error {
	ph := t.current()
	if ph == nil {
		return errBadPhase
	}
	if !contains(t.LegalActions(), action) {
		return errBadAction
	}

	switch action {
	case ActionStand:
		ph.done = true

	case ActionHit:
		ph.h.Add(t.shoe.Deal())
		if ph.h.IsBust() {
			ph.done = true
		}

	case ActionDouble:
		t.Bankroll -= ph.h.Bet
		ph.h.Bet *= 2
		ph.h.Doubled = true
		ph.h.Add(t.shoe.Deal())
		ph.done = true

	case ActionSurrender:
		ph.h.Surrendered = true
		ph.done = true

	case ActionSplit:
		second := ph.h.Cards[1]
		t.Bankroll -= t.bet
		nh := &playHand{h: &Hand{
			Cards:     []*Card{second},
			Bet:       t.bet,
			FromSplit: true,
			SplitAce:  second.IsAce(),
		}}
		ph.h.Cards = []*Card{ph.h.Cards[0]}
		ph.h.FromSplit = true
		ph.h.SplitAce = ph.h.Cards[0].IsAce()
		ph.h.Add(t.shoe.Deal())
		nh.h.Add(t.shoe.Deal())

		// La nouvelle main se place juste après l'active, comme à une vraie
		// table où l'on finit la première moitié avant de passer à la seconde.
		t.hands = append(t.hands, nil)
		copy(t.hands[t.active+2:], t.hands[t.active+1:])
		t.hands[t.active+1] = nh
	}

	ph.acted = true
	t.autoAdvance()
	return nil
}

// autoAdvance clôt les mains qui n'ont plus de décision à prendre et fait
// jouer le croupier quand le joueur a terminé.
func (t *Table) autoAdvance() {
	for {
		ph := t.current()
		if ph == nil {
			break
		}
		// Un As séparé ne reçoit qu'une carte, sauf règle contraire.
		if ph.h.SplitAce && !t.rules.HitSplitAces && len(ph.h.Cards) >= 2 {
			ph.done = true
		}
		// Un 21 n'a plus rien à gagner à tirer.
		if tot, _ := ph.h.Total(); tot >= 21 {
			ph.done = true
		}
		if !ph.done {
			return
		}
		t.active++
	}
	t.finish()
}

// finish fait jouer le croupier puis règle toutes les mains.
func (t *Table) finish() {
	t.revealed = true

	live := false
	for _, ph := range t.hands {
		if !ph.h.Surrendered && !ph.h.IsBust() {
			live = true
			break
		}
	}
	if live || t.side.Buster > 0 {
		playDealer(t.dealer, t.shoe, t.rules, nil)
	}

	dealerTotal, _ := t.dealer.Total()
	dealerBust := t.dealer.IsBust()

	for _, ph := range t.hands {
		switch {
		case ph.h.Surrendered:
			back := ph.h.Bet / 2
			t.Bankroll += back
			t.settleHand(ph, "abandon", back)
		case ph.h.IsBust():
			t.settleHand(ph, "saute", 0)
		case dealerBust:
			ret := ph.h.Bet * 2
			t.Bankroll += ret
			t.settleHand(ph, "gagne", ret)
		default:
			tot, _ := ph.h.Total()
			switch {
			case tot > dealerTotal:
				ret := ph.h.Bet * 2
				t.Bankroll += ret
				t.settleHand(ph, "gagne", ret)
			case tot < dealerTotal:
				t.settleHand(ph, "perdu", 0)
			default:
				t.Bankroll += ph.h.Bet
				t.settleHand(ph, "egalite", ph.h.Bet)
			}
		}
	}

	if dealerBust {
		t.note("Le croupier saute.")
	}
	t.resolveBuster()
	t.phase = PhaseDone
}

// settleHand enregistre le dénouement d'une main et son retour.
func (t *Table) settleHand(ph *playHand, result string, back float64) {
	ph.result = result
	ph.payout = back
	ph.done = true
	t.roundNet += back - ph.h.Bet
}

func (t *Table) resolveLuckyLadies(dealerBJ bool) {
	if t.side.LuckyLadies <= 0 {
		return
	}
	cards := t.hands[0].h.Cards
	m, label := EvalLuckyLadies(cards[0], cards[1], dealerBJ)
	t.addSide("Lucky Ladies", label, t.side.LuckyLadies, m)
}

func (t *Table) resolveBuster() {
	if t.side.Buster <= 0 {
		return
	}
	m, label := EvalBuster(len(t.dealer.Cards), t.dealer.IsBust())
	t.addSide("Buster Blackjack", label, t.side.Buster, m)
}

func (t *Table) resolveBusterNoDraw() {
	if t.side.Buster <= 0 {
		return
	}
	t.addSide("Buster Blackjack", "perdu", t.side.Buster, 0)
}

// addSide enregistre un pari annexe et crédite son retour.
func (t *Table) addSide(name, label string, stake, multiplier float64) {
	net := netSide(stake, multiplier)
	if net > 0 {
		t.Bankroll += stake + net
	}
	t.roundNet += net
	t.sideResults = append(t.sideResults, SideResult{
		Name:  name,
		Label: label,
		Stake: stake,
		Net:   net,
	})
}

func (t *Table) note(msg string) { t.messages = append(t.messages, msg) }

// current renvoie la main active, ou nil si le joueur a terminé.
func (t *Table) current() *playHand {
	if t.phase != PhasePlaying || t.active >= len(t.hands) {
		return nil
	}
	return t.hands[t.active]
}

// LegalActions énumère les actions jouables dans l'état courant.
func (t *Table) LegalActions() []string {
	switch t.phase {
	case PhaseInsurance:
		return []string{ActionInsure, ActionDecline}
	case PhaseBetting, PhaseDone:
		return nil
	}

	ph := t.current()
	if ph == nil {
		return nil
	}

	actions := []string{ActionHit, ActionStand}

	if len(ph.h.Cards) == 2 {
		if canDouble(ph.h, t.rules) && t.Bankroll >= ph.h.Bet {
			actions = append(actions, ActionDouble)
		}
		if ph.h.IsPair() && len(t.hands) < t.rules.MaxSplitHands &&
			canSplit(ph.h, t.rules) && t.Bankroll >= t.bet {
			actions = append(actions, ActionSplit)
		}
		// L'abandon n'est offert que sur la main initiale intacte.
		if t.rules.LateSurrender && !ph.acted && !ph.h.FromSplit && len(t.hands) == 1 {
			actions = append(actions, ActionSurrender)
		}
	}
	return actions
}

// Hint renvoie la décision que recommande la stratégie de base, pour
// l'affichage d'une aide au joueur.
func (t *Table) Hint() string {
	if t.phase == PhaseInsurance {
		return ActionDecline
	}
	ph := t.current()
	if ph == nil {
		return ""
	}
	switch Decide(ph.h, t.dealer.Cards[0], t.rules, len(t.hands)) {
	case Hit:
		return ActionHit
	case Stand:
		return ActionStand
	case Double:
		return ActionDouble
	case Split:
		return ActionSplit
	case Surrender:
		return ActionSurrender
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
