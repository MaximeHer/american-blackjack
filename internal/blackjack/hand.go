package blackjack

// Hand représente une main, celle du joueur comme celle du croupier.
//
// VERSION DE RÉFÉRENCE. Deux choix volontairement naïfs :
//   - les cartes sont dans un slice alimenté par append, donc chaque tirage
//     peut déclencher une réallocation et une recopie sur le tas ;
//   - le total est recalculé intégralement à chaque appel de Total(), alors
//     qu'il pourrait être maintenu en incrémental dans 3 octets.
//
// Total() est appelé plusieurs fois par décision (stratégie, test de bust,
// comparaison finale), ce qui en fait l'une des fonctions les plus chaudes
// du moteur.
type Hand struct {
	Cards       []Card
	Bet         float64
	Doubled     bool
	FromSplit   bool
	SplitAce    bool
	Surrendered bool
	Stood       bool
}

// Total renvoie le meilleur total de la main et indique si elle est souple,
// c'est-à-dire si un As y compte encore 11.
func (h *Hand) Total() (int, bool) {
	total := 0
	aces := 0
	for _, c := range h.Cards {
		total += c.Value()
		if c.IsAce() {
			aces++
		}
	}
	// Chaque As ramené de 11 à 1 retire 10 au total.
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total, aces > 0
}

// IsBust indique si la main dépasse 21.
func (h *Hand) IsBust() bool {
	t, _ := h.Total()
	return t > 21
}

// IsBlackjack reconnaît un blackjack : 21 en deux cartes sur la main initiale.
// Un 21 obtenu après un split n'est pas un blackjack et ne touche donc pas le
// paiement majoré, conformément aux règles de casino.
func (h *Hand) IsBlackjack() bool {
	if len(h.Cards) != 2 || h.FromSplit {
		return false
	}
	t, _ := h.Total()
	return t == 21
}

// IsPair indique si la main peut être séparée. Deux cartes de valeur 10 de
// rangs différents (un Roi et une Dame) forment une paire séparable.
func (h *Hand) IsPair() bool {
	return len(h.Cards) == 2 &&
		h.Cards[0].NormalizedRank() == h.Cards[1].NormalizedRank()
}

// Add ajoute une carte à la main.
func (h *Hand) Add(c Card) { h.Cards = append(h.Cards, c) }
