package blackjack

// Bornes du règlement, connues à la compilation.
const (
	// maxHandCards : une main ne peut excéder 21 cartes, cas théorique de
	// vingt-et-un As comptés 1. On réserve 22 par sécurité.
	maxHandCards = 22

	// maxHands : le règlement borne les séparations à quatre mains
	// simultanées. Voir Rules.MaxSplitHands, qui ne peut pas le dépasser.
	maxHands = 4
)

// Hand représente une main, celle du joueur comme celle du croupier.
//
// RANG 5 DU PROFIL : les cartes sont dans un TABLEAU FIXE, plus dans un slice.
//
// Un slice est un descripteur de 24 octets — pointeur, longueur, capacité — qui
// désigne un tableau alloué ailleurs. Le profil montrait que Hand.Add pesait
// 12,3 % des objets alloués, par croissance successive de ce tableau.
//
// Une main étant bornée à 21 cartes et une carte tenant dans un octet, les
// cartes se logent directement DANS la structure. Trois conséquences :
//
//  1. plus aucune allocation : une Hand déclarée en variable locale vit
//     entièrement sur la pile, et sa libération est gratuite ;
//  2. plus aucune indirection : lire une carte ne déréférence plus de pointeur,
//     les octets sont à un décalage constant depuis le début de la structure ;
//  3. la main entière tient dans une seule ligne de cache de 64 octets, donc
//     un seul défaut de cache suffit à la charger intégralement.
//
// Les champs sont ordonnés par taille décroissante (règle d'alignement), ce qui
// maintient la structure à 40 octets malgré les 22 octets de cartes.
type Hand struct {
	Bet   float64            // 8 o
	cards [maxHandCards]Card // 22 o, par valeur
	n     uint8              // nombre de cartes réellement présentes

	Doubled     bool
	FromSplit   bool
	SplitAce    bool
	Stood       bool
	Surrendered bool
}

// newHand construit une main à partir de cartes. Hors chemin chaud : le moteur
// remplit ses mains directement, sans passer par cette fonction.
func newHand(cards ...Card) Hand {
	var h Hand
	for _, c := range cards {
		h.Add(c)
	}
	return h
}

// Add ajoute une carte à la main.
//
// Aucune réallocation possible : la capacité est fixée à la compilation. Le
// dépassement est impossible par construction du jeu — une main qui atteindrait
// 22 cartes aurait déjà dépassé 21 depuis longtemps et le coup serait terminé.
func (h *Hand) Add(c Card) {
	h.cards[h.n] = c
	h.n++
}

// Len renvoie le nombre de cartes de la main.
func (h *Hand) Len() int { return int(h.n) }

// Card renvoie la i-ème carte de la main.
func (h *Hand) Card(i int) Card { return h.cards[i] }

// Cards renvoie une vue sur les cartes de la main.
//
// Ne produit aucune copie ni allocation, mais le slice renvoyé pointe DANS la
// structure : à n'utiliser que hors du chemin chaud, car le faire échapper
// forcerait la main entière sur le tas et annulerait tout le bénéfice.
func (h *Hand) Cards() []Card { return h.cards[:h.n] }

// keepFirst ne conserve que la première carte de la main, ce que fait une
// séparation sur la moitié qui reste en place.
func (h *Hand) keepFirst() { h.n = 1 }

// Total renvoie le meilleur total de la main et indique si elle est souple,
// c'est-à-dire si un As y compte encore 11.
func (h *Hand) Total() (int, bool) {
	countHandTotal()
	total := 0
	aces := 0
	for _, c := range h.cards[:h.n] {
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
	if h.n != 2 || h.FromSplit {
		return false
	}
	t, _ := h.Total()
	return t == 21
}

// IsPair indique si la main peut être séparée. Deux cartes de valeur 10 de
// rangs différents (un Roi et une Dame) forment une paire séparable.
func (h *Hand) IsPair() bool {
	if h.n != 2 {
		return false
	}
	a, b := h.cards[0], h.cards[1]
	// Deux cartes de valeur 10 forment une paire séparable même de rangs
	// différents : la comparaison porte donc sur la valeur, pas sur le rang.
	return a.SameRank(b) || (a.IsTenValue() && b.IsTenValue())
}

// Describe énumère les cartes de la main en clair, pour le journal narratif.
// Construit une chaîne, donc appelé uniquement sous garde de narration.
func (h *Hand) Describe() string {
	s := ""
	for i, c := range h.cards[:h.n] {
		if i > 0 {
			s += ", "
		}
		s += c.Label()
	}
	return s
}
