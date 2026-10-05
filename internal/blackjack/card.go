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
//   - les cartes sont de plus manipulées par pointeur ([]*Card), donc chacune
//     est allouée séparément sur le tas et se trouve n'importe où en mémoire :
//     parcourir une main déréférence autant de pointeurs qu'elle a de cartes,
//     sans aucune garantie de localité ;
//   - chaque comparaison de rang est une comparaison de chaînes.
type Card struct {
	Rank string
	Suit string
}

// Composition d'un jeu de 52 cartes.
var (
	AllRanks = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	AllSuits = []string{"Pique", "Coeur", "Carreau", "Trefle"}
)

// cardValues associe chaque rang à sa valeur au blackjack.
//
// VERSION DE RÉFÉRENCE : une map consultée à chaque évaluation de carte. Or
// Hand.Total() parcourt toutes les cartes de la main et est appelé plusieurs
// fois par décision — on paie donc un hachage de chaîne par carte et par
// appel. Le domaine est pourtant borné et connu à la compilation : un tableau
// indexé par le rang ferait le même travail sans hachage ni indirection.
var cardValues = map[string]int{
	"2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9,
	"10": 10, "J": 10, "Q": 10, "K": 10, "A": 11,
}

// straightRanks donne le rang ordinal servant à détecter une suite dans le
// pari 21+3. Même remarque que pour cardValues : une map là où un tableau
// suffirait.
var straightRanks = map[string]int{
	"2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9,
	"10": 10, "J": 11, "Q": 12, "K": 13, "A": 14,
}

// Value renvoie la valeur de la carte au blackjack, l'As comptant 11.
// La réduction de l'As à 1 est décidée au niveau de la main (voir Hand.Total).
func (c *Card) Value() int { return cardValues[c.Rank] }

// IsAce indique si la carte est un As.
func (c *Card) IsAce() bool { return c.Rank == "A" }

// IsTenValue indique si la carte vaut 10 (10, Valet, Dame, Roi).
func (c *Card) IsTenValue() bool { return !c.IsAce() && c.Value() == 10 }

// Color renvoie la couleur chromatique de la carte. Nécessaire pour départager
// une paire colorée d'une paire mixte dans le pari Perfect Pairs.
func (c *Card) Color() string {
	if c.Suit == "Coeur" || c.Suit == "Carreau" {
		return "Rouge"
	}
	return "Noir"
}

// StraightRank renvoie le rang ordinal pour la détection de suite, l'As valant
// 14. Il est aussi évalué comme 1 pour reconnaître la suite As-2-3 (voir
// isStraight).
func (c *Card) StraightRank() int { return straightRanks[c.Rank] }

// NormalizedRank regroupe 10, Valet, Dame et Roi sous la clé "10". Les tables
// de stratégie de base ne distinguent pas ces quatre rangs, qui valent tous 10
// et se séparent indifféremment.
func (c *Card) NormalizedRank() string {
	if c.IsTenValue() {
		return "10"
	}
	return c.Rank
}

// Label décrit la carte en clair, pour le journal narratif d'un coup.
//
// VERSION DE RÉFÉRENCE : cette concaténation est appelée pour chaque carte
// distribuée, y compris en simulation où personne ne lit le journal.
func (c *Card) Label() string { return c.Rank + " de " + c.Suit }
