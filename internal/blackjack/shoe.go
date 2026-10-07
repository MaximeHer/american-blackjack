package blackjack

import "math/rand"

// Shoe est le sabot de plusieurs jeux mélangés, muni d'une carte de coupe.
//
// Choix volontairement naïfs restants :
//
//  1. La distribution retire la carte de tête par re-slicing, ce qui interdit
//     de réutiliser le tableau sous-jacent et impose de réallouer le slice à
//     chaque rebattage. C'est le rang 3 du profil : un tableau fixe et un
//     curseur d'index supprimeront la dernière allocation.
//
// Déjà corrigé : le mélange, par Fisher-Yates en place (rang 1), et le stockage
// des cartes, désormais par valeur dans un bloc contigu (rang 2). Un sabot de
// 4 jeux occupe 208 octets sur 4 lignes de cache, contre 6 656 octets sur 104
// lignes et 208 objets dispersés dans la version de référence.
type Shoe struct {
	cards       []Card
	numDecks    int
	penetration float64
	cutAt       int
	rng         *rand.Rand

	Shuffles   int
	CardsDealt int
}

// NewShoe construit un sabot de numDecks jeux et le mélange. La pénétration
// est la fraction du sabot distribuée avant rebattage : 0.75 signifie que la
// carte de coupe est placée aux trois quarts.
func NewShoe(numDecks int, penetration float64, rng *rand.Rand) *Shoe {
	s := &Shoe{
		numDecks:    numDecks,
		penetration: penetration,
		rng:         rng,
	}
	s.Shuffle()
	return s
}

// Shuffle reconstruit le sabot complet, le mélange et repositionne la carte de
// coupe.
//
// RANG 1 DU PROFIL. Le mélange est désormais un Fisher-Yates en place.
//
// La version de référence tirait une carte au hasard puis la retirait du paquet.
// L'algorithme était correct et uniforme, mais chaque retrait décalait tous les
// éléments situés après l'index tiré. L'index étant uniforme, le nombre moyen de
// déplacements valait n(n-1)/4, soit 10 764 pour n = 208 — le comptage
// instrumenté en mesurait 10 756.
//
// Fisher-Yates obtient la même distribution en n-1 = 207 échanges sur place,
// sans jamais décaler quoi que ce soit : un facteur 52 sur le nombre de
// déplacements.
//
// La capacité est de plus réservée d'un coup, ce qui supprime les
// réallocations successives du slice au fil des append.
//
// Ce que ce palier NE fait PAS : les cartes restent allouées une par une
// derrière des pointeurs. C'est le rang 2 du profil, et les deux coûts sont
// mesurés séparément parce que le profil les distingue — ligne 62 pour
// l'allocation, ligne 80 pour le décalage.
func (s *Shoe) Shuffle() {
	size := s.numDecks * 52

	// Une seule allocation pour tout le sabot, au lieu de 208 objets distincts :
	// les cartes étant des octets, elles tiennent par valeur dans un bloc
	// contigu. Il ne reste que l'allocation du slice lui-même, que le rang 3
	// supprimera.
	cards := make([]Card, 0, size)
	for d := 0; d < s.numDecks; d++ {
		for r := uint8(0); r < rankCount; r++ {
			for su := uint8(0); su < suitCount; su++ {
				cards = append(cards, newCard(r, su))
			}
		}
	}

	// Fisher-Yates : on parcourt le tableau de la fin vers le début et on
	// échange chaque élément avec un élément tiré parmi lui-même et ceux qui le
	// précèdent. Chaque position est ainsi fixée définitivement en un échange,
	// et la distribution obtenue est uniforme sur les n! permutations.
	for i := len(cards) - 1; i > 0; i-- {
		j := s.rng.Intn(i + 1)
		cards[i], cards[j] = cards[j], cards[i]
		if Instrumented {
			// Un échange déplace un élément vers sa position définitive. Le
			// compteur reste donc comparable à celui de la version de
			// référence, qui comptait les décalages.
			countShuffleMoves(1)
		}
	}

	s.cards = cards
	// La coupe est placée de sorte qu'il reste (1 - penetration) du sabot
	// quand elle sort.
	s.cutAt = size - int(float64(size)*s.penetration)
	s.Shuffles++
}

// Deal distribue la carte suivante. Par sécurité, un sabot épuisé est rebattu,
// mais la carte de coupe doit normalement provoquer le rebattage bien avant.
func (s *Shoe) Deal() Card {
	if len(s.cards) == 0 {
		s.Shuffle()
	}
	c := s.cards[0]
	s.cards = s.cards[1:]
	s.CardsDealt++
	return c
}

// CutReached indique que la carte de coupe est sortie. Le sabot se rebat entre
// deux coups, jamais au milieu d'un coup.
func (s *Shoe) CutReached() bool { return len(s.cards) <= s.cutAt }

// Remaining renvoie le nombre de cartes encore dans le sabot.
func (s *Shoe) Remaining() int { return len(s.cards) }

// Size renvoie la taille nominale du sabot, soit 52 cartes par jeu.
func (s *Shoe) Size() int { return s.numDecks * 52 }
