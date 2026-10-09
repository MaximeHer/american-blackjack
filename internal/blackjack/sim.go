package blackjack

import (
	"math"
	"math/rand"
)

// Stats agrège les résultats d'une simulation complète.
type Stats struct {
	Rounds      int
	Hands       int
	MainWagered float64
	Action      float64
	MainNet     float64
	SideWagered float64
	SideNet     float64
	PlayerBJ    int
	DealerBJ    int
	// DealerPlayed compte les coups où le croupier a complété sa main. C'est
	// le bon dénominateur pour le taux de dépassement.
	DealerPlayed int
	DealerBust   int
	Shuffles     int
	CardsDealt   int

	// SumSqNet accumule le carré du résultat net de chaque coup, ce qui permet
	// de restituer la variance et l'écart-type sans conserver l'historique.
	SumSqNet float64
}

// HouseEdge renvoie l'avantage de la maison sur le jeu principal, en fraction
// de la mise initiale. Une valeur positive signifie que la maison gagne.
//
// C'est le chiffre qui sert d'oracle de non-régression : toute optimisation
// doit le laisser rigoureusement identique à seed égal.
func (st Stats) HouseEdge() float64 {
	if st.MainWagered == 0 {
		return 0
	}
	return -st.MainNet / st.MainWagered
}

// ElementOfRisk rapporte la perte à l'ensemble des sommes réellement engagées,
// splits et doubles compris, et non à la seule mise initiale.
func (st Stats) ElementOfRisk() float64 {
	if st.Action == 0 {
		return 0
	}
	return -st.MainNet / st.Action
}

// Variance renvoie la variance du résultat net par coup, exprimée en carrés
// d'unités de mise initiale.
func (st Stats) Variance() float64 {
	if st.Rounds < 2 {
		return 0
	}
	n := float64(st.Rounds)
	mean := st.MainNet / n
	return st.SumSqNet/n - mean*mean
}

// StdDev renvoie l'écart-type du résultat net par coup. Au blackjack il vaut
// environ 1,14 unité de mise, ce qui domine très largement l'avantage de la
// maison lui-même : c'est la raison pour laquelle il faut un très grand nombre
// de coups pour mesurer cet avantage avec précision.
func (st Stats) StdDev() float64 { return math.Sqrt(st.Variance()) }

// StdError renvoie l'erreur-type sur l'avantage de la maison mesuré, soit
// l'écart-type divisé par la racine du nombre de coups.
//
// C'est la grandeur qui dimensionne tout le protocole de mesure : pour
// résoudre l'avantage de la maison à 0,01 point près, il faut environ
// 1,3 x 10^8 coups par exécution. Le débit du moteur n'est donc pas un
// caprice de performance, c'est la condition de la précision statistique.
func (st Stats) StdError() float64 {
	if st.Rounds == 0 {
		return 0
	}
	return st.StdDev() / math.Sqrt(float64(st.Rounds))
}

// SideEdge renvoie l'avantage de la maison sur l'ensemble des paris annexes.
func (st Stats) SideEdge() float64 {
	if st.SideWagered == 0 {
		return 0
	}
	return -st.SideNet / st.SideWagered
}

// Add agrège un résultat de coup dans les statistiques.
func (st *Stats) Add(res RoundResult) {
	st.Rounds++
	st.Hands += res.Hands
	st.MainWagered += res.MainWagered
	st.Action += res.Action
	st.MainNet += res.MainNet
	st.SumSqNet += res.MainNet * res.MainNet
	st.SideWagered += res.SideWagered
	st.SideNet += res.SideNet
	if res.PlayerBJ {
		st.PlayerBJ++
	}
	if res.DealerBJ {
		st.DealerBJ++
	}
	if res.DealerPlayed {
		st.DealerPlayed++
	}
	if res.DealerBust {
		st.DealerBust++
	}
}

// Simulate joue n coups et renvoie les statistiques agrégées.
//
// Le générateur pseudo-aléatoire est initialisé par seed, ce qui garantit la
// reproductibilité stricte exigée par le protocole de mesure : deux exécutions
// de même seed produisent exactement les mêmes cartes, les mêmes décisions et
// le même avantage de la maison.
//
// VERSION DE RÉFÉRENCE : exécution strictement séquentielle, sur un seul
// coeur, avec le générateur math/rand de la bibliothèque standard.
func Simulate(n int, seed int64, r Rules, bet float64, sb SideBets) Stats {
	rng := rand.New(rand.NewSource(seed))
	shoe := NewShoe(r.NumDecks, r.Penetration, rng)

	var st Stats
	for i := 0; i < n; i++ {
		// Le sabot se rebat entre deux coups, jamais au milieu d'un coup.
		if shoe.CutReached() {
			shoe.Shuffle()
		}
		// nil : la boucle de reference ne produit aucun recit.
		st.Add(PlayRound(shoe, r, bet, sb, nil))
	}
	st.Shuffles = shoe.Shuffles
	st.CardsDealt = shoe.CardsDealt
	return st
}

// Merge agrège les statistiques d'un autre lot.
//
// L'opération est associative sur les compteurs entiers, donc l'ordre de
// fusion ne change rien pour eux. Elle ne l'est PAS sur les sommes flottantes,
// l'addition en virgule flottante perdant de la précision différemment selon
// l'ordre : c'est pourquoi SimulateParallel fusionne toujours dans l'ordre des
// indices de worker, jamais dans l'ordre d'arrivée.
func (st *Stats) Merge(o Stats) {
	st.Rounds += o.Rounds
	st.Hands += o.Hands
	st.MainWagered += o.MainWagered
	st.Action += o.Action
	st.MainNet += o.MainNet
	st.SumSqNet += o.SumSqNet
	st.SideWagered += o.SideWagered
	st.SideNet += o.SideNet
	st.PlayerBJ += o.PlayerBJ
	st.DealerBJ += o.DealerBJ
	st.DealerPlayed += o.DealerPlayed
	st.DealerBust += o.DealerBust
	st.Shuffles += o.Shuffles
	st.CardsDealt += o.CardsDealt
}
