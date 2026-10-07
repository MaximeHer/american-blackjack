package blackjack

// Card encode une carte dans un seul octet : 4 bits de rang, 2 bits d'enseigne.
//
// RANG 2 DU PROFIL. C'est le palier structurant du projet. Il attaque les deux
// lignes les plus coûteuses du programme d'un seul coup :
//
//   - `card.go:53` dans la version de référence — `return cardValues[c.Rank]` —
//     qui pesait 1,57 s, soit 20,9 % du temps total à elle seule. Une
//     consultation de `map[string]float64` par carte, et `Hand.Total` en faisait
//     29,17 par coup.
//   - `shoe.go:62` — `append(pool, &Card{Rank: r, Suit: su})` — qui pesait
//     640 ms et allouait 208 objets par rebattage.
//
// Les deux disparaissent pour la même raison : un rang numérique indexe un
// tableau sans hachage, et une carte d'un octet se stocke par valeur sans
// allocation.
//
// # Gain physique attendu
//
// La version de référence occupait 32 octets par carte — deux en-têtes de
// `string` de 16 octets — et les manipulait par pointeur, chacune allouée
// séparément dans le tas. Un sabot de 4 jeux représentait 6 656 octets répartis
// sur 104 lignes de cache, plus 208 objets dispersés.
//
// Compacté, ce même sabot occupe 208 octets contigus, soit 4 lignes de cache :
// il tient intégralement dans le L1 de 384 Ko, et son parcours devient
// strictement séquentiel, motif que le préchargeur matériel reconnaît.
//
// Facteur 32 sur la représentation, et 64 cartes par ligne de cache au lieu de 2.
//
// # Pourquoi ce palier n'est pas scindable
//
// On ne peut pas indexer un tableau par le rang d'une carte sans que ce rang
// soit numérique. La compaction de la carte et le remplacement de la map sont
// donc un seul et même changement. Le palier intermédiaire « map → switch »
// envisagé dans la feuille de route initiale a été abandonné : il aurait
// atténué le problème au lieu de le supprimer.
type Card uint8

// Indices de rang, de 0 à 12. L'ordre est celui du jeu, du 2 à l'As.
const (
	rank2 uint8 = iota
	rank3
	rank4
	rank5
	rank6
	rank7
	rank8
	rank9
	rank10
	rankJack
	rankQueen
	rankKing
	rankAce
	rankCount
)

// Indices d'enseigne, de 0 à 3.
const (
	suitPique uint8 = iota
	suitCoeur
	suitCarreau
	suitTrefle
	suitCount
)

// suitShift est le nombre de bits réservés à l'enseigne dans l'octet. Le rang
// occupe les bits supérieurs.
const suitShift = 2

// suitMask isole les bits d'enseigne.
const suitMask = Card(suitCount - 1)

// Noms d'affichage, indexés par leur code. Consultés hors du chemin mesuré.
var (
	rankNames = [rankCount]string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	suitNames = [suitCount]string{"Pique", "Coeur", "Carreau", "Trefle"}
)

// Composition d'un jeu de 52 cartes, exposée pour l'affichage et les tests.
var (
	AllRanks = rankNames[:]
	AllSuits = suitNames[:]
)

// rankValues donne la valeur au blackjack de chaque rang, l'As comptant 11.
//
// Tableau plat indexé par le rang : une lecture mémoire à décalage constant, à
// comparer au hachage de chaîne qu'il remplace. La table de 13 octets tient dans
// une fraction de ligne de cache et y reste.
var rankValues = [rankCount]uint8{
	rank2: 2, rank3: 3, rank4: 4, rank5: 5, rank6: 6,
	rank7: 7, rank8: 8, rank9: 9, rank10: 10,
	rankJack: 10, rankQueen: 10, rankKing: 10,
	rankAce: 11,
}

// rankStraight donne le rang ordinal servant à détecter une suite dans le pari
// 21+3, l'As valant 14.
var rankStraight = [rankCount]uint8{
	rank2: 2, rank3: 3, rank4: 4, rank5: 5, rank6: 6,
	rank7: 7, rank8: 8, rank9: 9, rank10: 10,
	rankJack: 11, rankQueen: 12, rankKing: 13,
	rankAce: 14,
}

// rankNormalized regroupe 10, Valet, Dame et Roi sous la clé "10", les tables de
// stratégie ne distinguant pas ces quatre rangs.
//
// Reste une chaîne parce que la table de stratégie est encore indexée par des
// clés textuelles. Le tableau évite au moins d'en reconstruire une : c'est le
// rang 4 du profil qui supprimera le problème à la racine.
var rankNormalized = [rankCount]string{
	rank2: "2", rank3: "3", rank4: "4", rank5: "5", rank6: "6",
	rank7: "7", rank8: "8", rank9: "9", rank10: "10",
	rankJack: "10", rankQueen: "10", rankKing: "10",
	rankAce: "A",
}

// rankIsRed indique si une enseigne est rouge, pour départager une paire colorée
// d'une paire mixte dans le pari Perfect Pairs.
var suitIsRed = [suitCount]bool{
	suitPique: false, suitCoeur: true, suitCarreau: true, suitTrefle: false,
}

// newCard assemble une carte depuis son rang et son enseigne.
func newCard(rank, suit uint8) Card { return Card(rank)<<suitShift | Card(suit) }

// rank renvoie le code de rang, de 0 (le 2) à 12 (l'As).
func (c Card) rank() uint8 { return uint8(c >> suitShift) }

// suit renvoie le code d'enseigne, de 0 à 3.
func (c Card) suit() uint8 { return uint8(c & suitMask) }

// Value renvoie la valeur de la carte au blackjack, l'As comptant 11. La
// réduction de l'As à 1 est décidée au niveau de la main (voir Hand.Total).
func (c Card) Value() int {
	countCardValue()
	return int(rankValues[c.rank()])
}

// IsAce indique si la carte est un As.
func (c Card) IsAce() bool { return c.rank() == rankAce }

// IsTenValue indique si la carte vaut 10 : le 10, le Valet, la Dame ou le Roi.
// Ces quatre rangs sont consécutifs dans l'encodage, d'où la comparaison
// d'intervalle.
func (c Card) IsTenValue() bool {
	r := c.rank()
	return r >= rank10 && r <= rankKing
}

// IsRed indique si la carte est rouge.
func (c Card) IsRed() bool { return suitIsRed[c.suit()] }

// StraightRank renvoie le rang ordinal pour la détection de suite, l'As valant
// 14. Il est aussi évalué comme 1 pour reconnaître la suite As-2-3.
func (c Card) StraightRank() int { return int(rankStraight[c.rank()]) }

// NormalizedRank regroupe les quatre rangs de valeur 10 sous la clé "10".
func (c Card) NormalizedRank() string { return rankNormalized[c.rank()] }

// SameRank indique si deux cartes ont le même rang, quelle que soit l'enseigne.
func (c Card) SameRank(o Card) bool { return c.rank() == o.rank() }

// SameSuit indique si deux cartes ont la même enseigne.
func (c Card) SameSuit(o Card) bool { return c.suit() == o.suit() }

// RankName renvoie le nom du rang, pour l'affichage.
func (c Card) RankName() string { return rankNames[c.rank()] }

// SuitName renvoie le nom de l'enseigne, pour l'affichage.
func (c Card) SuitName() string { return suitNames[c.suit()] }

// Label décrit la carte en clair, pour le journal narratif d'un coup. Construit
// une chaîne, donc à n'appeler que hors du chemin mesuré.
func (c Card) Label() string { return c.RankName() + " de " + c.SuitName() }

// cardFromNames construit une carte depuis ses libellés.
//
// Hors du chemin mesuré : sert aux tests et à toute conversion depuis une saisie
// textuelle. Renvoie false si le rang ou l'enseigne est inconnu.
func cardFromNames(rank, suit string) (Card, bool) {
	r, ok := indexOf(rankNames[:], rank)
	if !ok {
		return 0, false
	}
	s, ok := indexOf(suitNames[:], suit)
	if !ok {
		return 0, false
	}
	return newCard(uint8(r), uint8(s)), true
}

func indexOf(list []string, v string) (int, bool) {
	for i, x := range list {
		if x == v {
			return i, true
		}
	}
	return 0, false
}
