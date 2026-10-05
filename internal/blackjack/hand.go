package blackjack

// Hand représente une main, celle du joueur comme celle du croupier.
//
// VERSION DE RÉFÉRENCE. Trois choix volontairement naïfs :
//
//  1. Les cartes sont un slice de POINTEURS, alimenté par append. Chaque
//     tirage peut réallouer et recopier, et chaque lecture d'une carte
//     déréférence un pointeur vers une zone du tas sans rapport avec ses
//     voisines. La main n'a aucune localité.
//
//  2. Le total est recalculé intégralement à chaque appel de Total(), alors
//     qu'il pourrait être maintenu en incrémental dans trois octets. Total()
//     est appelé plusieurs fois par décision — stratégie, test de
//     dépassement, comparaison finale — ce qui en fait la fonction la plus
//     chaude du moteur.
//
//  3. L'ordre des champs est quelconque : les booléens sont intercalés entre
//     les champs de 8 octets, ce qui force le compilateur à insérer du
//     remplissage. Go ne réordonne jamais les champs d'une structure, donc ce
//     gaspillage est réel et mesurable par unsafe.Sizeof. Regrouper les
//     booléens en fin de structure suffirait à le supprimer.
type Hand struct {
	Doubled     bool
	Bet         float64
	FromSplit   bool
	Cards       []*Card
	SplitAce    bool
	Stood       bool
	Surrendered bool
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
func (h *Hand) Add(c *Card) { h.Cards = append(h.Cards, c) }

// Describe énumère les cartes de la main en clair, pour le journal narratif.
//
// VERSION DE RÉFÉRENCE : construit une chaîne par appel, y compris en
// simulation où le journal n'est jamais lu.
func (h *Hand) Describe() string {
	s := ""
	for i, c := range h.Cards {
		if i > 0 {
			s += ", "
		}
		s += c.Label()
	}
	return s
}
