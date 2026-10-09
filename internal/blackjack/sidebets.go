package blackjack

import "sort"

// SideBets porte les montants misés sur chaque pari annexe. Un montant nul
// signifie que le pari n'est pas joué, et son évaluation est alors sautée.
type SideBets struct {
	PerfectPairs   float64 `json:"perfectPairs"`
	TwentyOnePlus3 float64 `json:"twentyOnePlus3"`
	LuckyLadies    float64 `json:"luckyLadies"`
	Buster         float64 `json:"buster"`
}

// Total renvoie la somme engagée sur les paris annexes pour un coup.
func (sb SideBets) Total() float64 {
	return sb.PerfectPairs + sb.TwentyOnePlus3 + sb.LuckyLadies + sb.Buster
}

// Any indique si au moins un pari annexe est joué.
func (sb SideBets) Any() bool { return sb.Total() > 0 }

// SideOutcome identifie la combinaison obtenue sur un pari annexe.
//
// PALIER 2. La version de référence identifiait chaque combinaison par une
// chaîne, puis consultait une map[string]float64 pour en tirer le gain. Deux
// coûts se cumulaient sur le hot path : le hachage de la chaîne, et la
// manipulation de la chaîne elle-même, retournée à chaque évaluation même
// lorsque personne ne la lisait.
//
// Le domaine est pourtant borné et connu à la compilation : dix-neuf
// combinaisons en tout. Un entier suffit à les désigner, les gains se lisent
// dans un tableau plat indexé par cet entier, et le libellé lisible ne se
// calcule que lorsqu'on en a besoin — c'est-à-dire pour la narration, hors du
// chemin mesuré.
type SideOutcome uint8

const (
	// OutcomeLose vaut zéro, ce qui en fait la valeur par défaut d'un
	// SideOutcome non initialisé. C'est volontaire : un oubli se traduit par un
	// pari perdu, jamais par un gain fortuit.
	OutcomeLose SideOutcome = iota

	// Perfect Pairs
	PPPerfect // même rang et même enseigne
	PPColored // même rang, même couleur chromatique, enseignes différentes
	PPMixed   // même rang, couleurs chromatiques différentes

	// 21+3
	TPSuitedTrips // trois cartes identiques de même enseigne
	TPStraightFlush
	TPTrips
	TPStraight
	TPFlush

	// Lucky Ladies
	LLQueensHeartsBJ // deux Dames de coeur et blackjack du croupier
	LLQueensHearts
	LLMatched20 // 20 formé de deux cartes rigoureusement identiques
	LLSuited20  // 20 de même enseigne
	LLAny20

	// Buster Blackjack, selon le nombre de cartes de la main sautée
	Buster3
	Buster4
	Buster5
	Buster6
	Buster7
	Buster8Plus

	outcomeCount
)

// sideMultipliers donne le gain net de chaque combinaison, en multiplicateur du
// montant misé. Tableau plat indexé par la combinaison : pas de hachage, pas
// d'indirection.
var sideMultipliers = [outcomeCount]float64{
	OutcomeLose: 0,

	PPPerfect: 25,
	PPColored: 12,
	PPMixed:   6,

	TPSuitedTrips:   100,
	TPStraightFlush: 40,
	TPTrips:         30,
	TPStraight:      10,
	TPFlush:         5,

	LLQueensHeartsBJ: 1000,
	LLQueensHearts:   200,
	LLMatched20:      25,
	LLSuited20:       10,
	LLAny20:          4,

	Buster3:     2,
	Buster4:     2,
	Buster5:     4,
	Buster6:     18,
	Buster7:     50,
	Buster8Plus: 250,
}

// sideLabels donne le libellé lisible de chaque combinaison. Consulté
// uniquement pour la narration, donc hors du chemin mesuré.
var sideLabels = [outcomeCount]string{
	OutcomeLose: "perdu",

	PPPerfect: "paire parfaite",
	PPColored: "paire colorée",
	PPMixed:   "paire mixte",

	TPSuitedTrips:   "brelan de même enseigne",
	TPStraightFlush: "quinte flush",
	TPTrips:         "brelan",
	TPStraight:      "quinte",
	TPFlush:         "couleur",

	LLQueensHeartsBJ: "deux Dames de coeur et blackjack du croupier",
	LLQueensHearts:   "deux Dames de coeur",
	LLMatched20:      "20 de deux cartes identiques",
	LLSuited20:       "20 de même enseigne",
	LLAny20:          "20",

	Buster3:     "croupier sauté en 3 cartes",
	Buster4:     "croupier sauté en 4 cartes",
	Buster5:     "croupier sauté en 5 cartes",
	Buster6:     "croupier sauté en 6 cartes",
	Buster7:     "croupier sauté en 7 cartes",
	Buster8Plus: "croupier sauté en 8 cartes ou plus",
}

// queenHearts est la Dame de coeur, seule carte nommée du règlement : le gain
// maximal de Lucky Ladies exige une paire de celles-ci.
//
// Une carte tenant dans un octet, la comparaison devient une égalité d'entiers,
// là où la version de référence comparait deux chaînes de rang et deux chaînes
// d'enseigne.
var queenHearts = newCard(rankQueen, suitCoeur)

// Multiplier renvoie le gain net, en multiple de la mise. Zéro signifie perdu.
func (o SideOutcome) Multiplier() float64 { return sideMultipliers[o] }

// Label renvoie le libellé lisible de la combinaison. À n'appeler que hors du
// chemin mesuré.
func (o SideOutcome) Label() string { return sideLabels[o] }

// Won indique si la combinaison est gagnante.
func (o SideOutcome) Won() bool { return o != OutcomeLose }

// EvalPerfectPairs évalue le pari Perfect Pairs sur les deux premières cartes
// du joueur.
func EvalPerfectPairs(a, b Card) SideOutcome {
	if !a.SameRank(b) {
		return OutcomeLose
	}
	switch {
	case a.SameSuit(b):
		return PPPerfect
	case a.IsRed() == b.IsRed():
		return PPColored
	default:
		return PPMixed
	}
}

// EvalTwentyOnePlus3 évalue le pari 21+3 : les deux cartes du joueur et la
// carte visible du croupier forment une main de poker à trois cartes.
func EvalTwentyOnePlus3(a, b, up Card) SideOutcome {
	flush := a.SameSuit(b) && b.SameSuit(up)
	trips := a.SameRank(b) && b.SameRank(up)
	straight := isStraight(a, b, up)

	switch {
	case trips && flush:
		return TPSuitedTrips
	case straight && flush:
		return TPStraightFlush
	case trips:
		return TPTrips
	case straight:
		return TPStraight
	case flush:
		return TPFlush
	}
	return OutcomeLose
}

// isStraight teste trois cartes pour une suite. L'As est évalué deux fois,
// comme 14 pour reconnaître Dame-Roi-As et comme 1 pour reconnaître As-2-3.
func isStraight(cards ...Card) bool {
	high := make([]int, 0, len(cards))
	low := make([]int, 0, len(cards))
	for _, c := range cards {
		r := c.StraightRank()
		high = append(high, r)
		if r == 14 {
			low = append(low, 1)
		} else {
			low = append(low, r)
		}
	}
	return consecutive(high) || consecutive(low)
}

// consecutive indique si les valeurs forment une séquence strictement
// croissante de pas 1 après tri.
func consecutive(v []int) bool {
	s := make([]int, len(v))
	copy(s, v)
	sort.Ints(s)
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1]+1 {
			return false
		}
	}
	return true
}

// EvalLuckyLadies évalue le pari Lucky Ladies, qui paie si les deux premières
// cartes du joueur totalisent 20. Le gain maximal combine une paire de Dames de
// coeur et un blackjack du croupier.
func EvalLuckyLadies(a, b Card, dealerBJ bool) SideOutcome {
	// Total de deux cartes calculé directement : construire une Hand pour
	// deux cartes coûterait 40 octets de pile pour rien.
	total := a.Value() + b.Value()
	if total > 21 && (a.IsAce() || b.IsAce()) {
		total -= 10 // un As ramené de 11 à 1
	}
	if total != 20 {
		return OutcomeLose
	}
	queenOfHearts := a == queenHearts && b == queenHearts
	switch {
	case queenOfHearts && dealerBJ:
		return LLQueensHeartsBJ
	case queenOfHearts:
		return LLQueensHearts
	case a == b:
		return LLMatched20
	case a.SameSuit(b):
		return LLSuited20
	default:
		return LLAny20
	}
}

// EvalBuster évalue le pari Buster Blackjack, qui paie quand le croupier saute,
// d'autant plus que sa main compte de cartes.
func EvalBuster(dealerCardCount int, dealerBusted bool) SideOutcome {
	if !dealerBusted {
		return OutcomeLose
	}
	switch {
	case dealerCardCount <= 3:
		return Buster3
	case dealerCardCount == 4:
		return Buster4
	case dealerCardCount == 5:
		return Buster5
	case dealerCardCount == 6:
		return Buster6
	case dealerCardCount == 7:
		return Buster7
	default:
		return Buster8Plus
	}
}

// netSide convertit une combinaison en résultat net : si elle est perdante, la
// mise est perdue.
func netSide(stake float64, o SideOutcome) float64 {
	if !o.Won() {
		return -stake
	}
	return stake * o.Multiplier()
}
