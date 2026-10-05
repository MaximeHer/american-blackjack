package blackjack

import "math/rand"

// Shoe est le sabot de plusieurs jeux mélangés, muni d'une carte de coupe.
//
// VERSION DE RÉFÉRENCE. Deux choix volontairement naïfs :
//   - la distribution retire la carte de tête du slice (s.cards = s.cards[1:]),
//     ce qui fait avancer le pointeur mais interdit de réutiliser le tableau
//     sous-jacent ;
//   - chaque rebattage reconstruit intégralement le sabot avec append, donc
//     alloue 208 Card de 32 octets, soit environ 6,6 Ko sur le tas, et ce
//     plusieurs milliers de fois par simulation.
//
// La version optimisée gardera un tableau fixe [208]uint8 et un simple curseur
// d'index, ce qui supprime toute allocation et donne un accès strictement
// séquentiel, idéal pour le préchargement matériel du cache.
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

// Shuffle reconstruit le sabot complet, le mélange par Fisher-Yates et
// repositionne la carte de coupe.
func (s *Shoe) Shuffle() {
	cards := []Card{}
	for d := 0; d < s.numDecks; d++ {
		for _, r := range AllRanks {
			for _, su := range AllSuits {
				cards = append(cards, Card{Rank: r, Suit: su})
			}
		}
	}
	s.rng.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	s.cards = cards
	// La coupe est placée de sorte qu'il reste (1 - penetration) du sabot
	// quand elle sort.
	s.cutAt = len(cards) - int(float64(len(cards))*s.penetration)
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
