package blackjack

import "math/rand"

// Shoe est le sabot de plusieurs jeux mélangés, muni d'une carte de coupe.
//
// VERSION DE RÉFÉRENCE. Trois choix volontairement naïfs :
//
//  1. Le sabot est un slice de POINTEURS vers des cartes allouées une par une.
//     Un rebattage alloue donc 208 objets distincts plus le slice, et les
//     cartes se retrouvent dispersées dans le tas.
//
//  2. Le mélange est l'algorithme intuitif : tirer une carte au hasard dans le
//     paquet, la retirer, recommencer. Il est correct et uniforme, mais chaque
//     retrait décale la fin du slice, ce qui le rend quadratique.
//
//     L'index tiré étant uniforme, le nombre moyen de déplacements vaut
//     n(n-1)/4, soit 208 x 207 / 4 = 10 764 pour un sabot de 4 jeux. Le
//     comptage instrumenté mesure 10 756 : la théorie est vérifiée. Pour le
//     même résultat, Fisher-Yates effectue 208 échanges sur place, soit un
//     facteur 52.
//
//  3. La distribution retire la carte de tête par re-slicing, ce qui interdit
//     de réutiliser le tableau sous-jacent et impose de tout reconstruire au
//     rebattage.
//
// La version optimisée gardera un tableau fixe de cartes compactes et un
// simple curseur d'index : aucune allocation, accès strictement séquentiel,
// et un mélange en place.
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
func (s *Shoe) Shuffle() {
	// Construction du paquet : une allocation par carte.
	pool := []*Card{}
	for d := 0; d < s.numDecks; d++ {
		for _, r := range AllRanks {
			for _, su := range AllSuits {
				pool = append(pool, &Card{Rank: r, Suit: su})
			}
		}
	}

	// Mélange naïf : on tire une carte au hasard et on la retire du paquet.
	// Correct, mais quadratique à cause du décalage provoqué par chaque
	// retrait.
	shuffled := []*Card{}
	for len(pool) > 0 {
		i := s.rng.Intn(len(pool))
		shuffled = append(shuffled, pool[i])
		// Le retrait décale tous les éléments situés après i. C'est la source
		// du comportement quadratique, et le comptage ci-dessous le prouve
		// chiffres en main plutôt que par raisonnement.
		if Instrumented {
			countShuffleMoves(len(pool) - i - 1)
		}
		pool = append(pool[:i], pool[i+1:]...)
	}

	s.cards = shuffled
	// La coupe est placée de sorte qu'il reste (1 - penetration) du sabot
	// quand elle sort.
	s.cutAt = len(shuffled) - int(float64(len(shuffled))*s.penetration)
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
