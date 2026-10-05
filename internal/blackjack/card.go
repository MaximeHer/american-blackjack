package blackjack

// Card représente une carte du sabot.
//
// VERSION DE RÉFÉRENCE (baseline). Le rang et la couleur sont stockés sous
// forme de chaînes de caractères : c'est la représentation qu'on écrit
// spontanément, et c'est la première source de lenteur du moteur.
//
// Coût physique réel de ce choix, à mesurer et à documenter :
//   - unsafe.Sizeof(Card) == 32 octets, soit deux en-têtes de string de
//     16 octets (pointeur + longueur), alors qu'une carte tient dans un
//     seul octet (4 bits de rang, 2 bits de couleur) ;
//   - une ligne de cache de 64 octets ne contient donc que 2 cartes, contre
//     64 avec une représentation compacte ;
//   - chaque comparaison de rang est une comparaison de chaînes : appel de
//     fonction et déréférencement de pointeur, au lieu d'une comparaison
//     d'entiers en un cycle.
type Card struct {
	Rank string
	Suit string
}

// Composition d'un jeu de 52 cartes.
var (
	AllRanks = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	AllSuits = []string{"Pique", "Coeur", "Carreau", "Trefle"}
)

// Value renvoie la valeur de la carte au blackjack, l'As comptant 11.
// La réduction de l'As à 1 est décidée au niveau de la main, pas de la carte
// (voir Hand.Total).
func (c Card) Value() int {
	switch c.Rank {
	case "A":
		return 11
	case "10", "J", "Q", "K":
		return 10
	case "9":
		return 9
	case "8":
		return 8
	case "7":
		return 7
	case "6":
		return 6
	case "5":
		return 5
	case "4":
		return 4
	case "3":
		return 3
	case "2":
		return 2
	}
	return 0
}

// IsAce indique si la carte est un As.
func (c Card) IsAce() bool { return c.Rank == "A" }

// IsTenValue indique si la carte vaut 10 (10, Valet, Dame, Roi).
func (c Card) IsTenValue() bool { return !c.IsAce() && c.Value() == 10 }

// Color renvoie la couleur chromatique de la carte. Nécessaire pour départager
// une paire colorée d'une paire mixte dans le pari Perfect Pairs.
func (c Card) Color() string {
	if c.Suit == "Coeur" || c.Suit == "Carreau" {
		return "Rouge"
	}
	return "Noir"
}

// StraightRank renvoie le rang ordinal servant à détecter une suite dans le
// pari 21+3. L'As vaut 14 ici ; il est aussi évalué comme 1 pour reconnaître
// la suite As-2-3 (voir isStraight).
func (c Card) StraightRank() int {
	switch c.Rank {
	case "A":
		return 14
	case "K":
		return 13
	case "Q":
		return 12
	case "J":
		return 11
	}
	return c.Value()
}

// NormalizedRank regroupe 10, Valet, Dame et Roi sous la clé "10". Les tables
// de stratégie de base ne distinguent pas ces quatre rangs, qui valent tous 10
// et se séparent indifféremment.
func (c Card) NormalizedRank() string {
	if c.IsTenValue() {
		return "10"
	}
	return c.Rank
}
