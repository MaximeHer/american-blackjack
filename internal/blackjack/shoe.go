package blackjack

import "math/rand"

// Shoe est le sabot de plusieurs jeux mélangés, muni d'une carte de coupe.
//
// Choix volontairement naïfs restants :
//
//  1. Le sabot est un slice de POINTEURS vers des cartes allouées une par une.
//     Un rebattage alloue donc 208 objets distincts plus le slice, et les
//     cartes se retrouvent dispersées dans le tas.
//
//  2. La distribution retire la carte de tête par re-slicing, ce qui interdit
//     de réutiliser le tableau sous-jacent et impose de tout reconstruire au
//     rebattage.
//
// Le mélange, lui, a été corrigé au rang 1 du profil : voir Shuffle.
//
// La version optimisée gardera un tableau fixe de cartes compactes et un
// simple curseur d'index : aucune allocation, accès strictement séquentiel.
type Shoe struct {
	cards       []*Card
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

	// Capacité réservée d'un coup : plus aucune réallocation pendant le
	// remplissage. L'allocation d'une carte par carte subsiste, elle sera
	// traitée au rang 2.
	cards := make([]*Card, 0, size)
	for d := 0; d < s.numDecks; d++ {
		for _, r := range AllRanks {
			for _, su := range AllSuits {
				cards = append(cards, &Card{Rank: r, Suit: su})
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
func (s *Shoe) Deal() *Card {
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
