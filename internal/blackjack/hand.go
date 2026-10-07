package blackjack

// Hand représente une main, celle du joueur comme celle du croupier.
//
// RANG 2 DU PROFIL : les cartes sont désormais stockées PAR VALEUR. Chacune
// occupant un octet, un slice de cartes est un bloc d'octets contigu, et
// parcourir une main ne déréférence plus rien. La localité est rétablie.
//
// Choix volontairement naïfs restants :
//
//  1. Le slice est alimenté par append, donc un tirage peut encore réallouer.
//     Une main ne pouvant excéder 21 cartes, un tableau fixe suffirait. C'est
//     le rang 5 du profil.
//
//  2. Le total est recalculé intégralement à chaque appel de Total(), alors
//     qu'il pourrait être maintenu en incrémental. Le profil a toutefois montré
//     que 1,43 s des 1,68 s de Total venaient de Card.Value, c'est-à-dire de la
//     consultation de map que ce palier supprime : le gain restant sera donc
//     bien plus faible qu'escompté, d'où le déclassement de ce palier en
//     dernière position.
//
// Les champs sont ordonnés par taille DÉCROISSANTE, conformément à la règle
// d'alignement : le processeur exige que chaque champ débute à une adresse
// multiple de sa taille, et Go ne réordonne jamais les champs. Des booléens
// intercalés entre des champs de 8 octets forcent donc le compilateur à insérer
// du remplissage invisible.
//
// Avant réordonnancement : 56 octets pour 37 octets utiles, soit 19 octets
// perdus (34 %). Le gaspillage franchissait de plus une classe de taille de
// l'allocateur Go, qui servait 64 octets au lieu de 48.
type Hand struct {
	Cards       []Card  // 24 o
	Bet         float64 // 8 o
	Doubled     bool    // les booléens regroupés en fin de structure
	FromSplit   bool
	SplitAce    bool
	Stood       bool
	Surrendered bool
}

// Total renvoie le meilleur total de la main et indique si elle est souple,
// c'est-à-dire si un As y compte encore 11.
func (h *Hand) Total() (int, bool) {
	countHandTotal()
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
	if len(h.Cards) != 2 {
		return false
	}
	a, b := h.Cards[0], h.Cards[1]
	// Deux cartes de valeur 10 forment une paire séparable même de rangs
	// différents : la comparaison porte donc sur la valeur, pas sur le rang.
	return a.SameRank(b) || (a.IsTenValue() && b.IsTenValue())
}

// Add ajoute une carte à la main.
func (h *Hand) Add(c Card) { h.Cards = append(h.Cards, c) }

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
