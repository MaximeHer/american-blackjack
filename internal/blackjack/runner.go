package blackjack

import "math/rand"

// Runner déroule une simulation par lots, en conservant son état entre deux
// lots.
//
// CE TYPE EST HORS DU CHEMIN MESURÉ. Sa boucle duplique volontairement celle de
// Simulate au lieu de l'instrumenter : découper la boucle de référence pour
// émettre de la progression en falsifierait la mesure, et c'est précisément ce
// que le protocole interdit.
//
// Conséquence à assumer et à afficher : une exécution déroulée par lots n'est
// PAS une mesure de référence. Le relevé des compteurs entre chaque lot, et la
// rupture du régime permanent qu'il provoque, la perturbent légèrement. Le
// chiffre qui fait foi reste celui d'une exécution d'un seul bloc, mesurée en
// ligne de commande.
//
// Son usage légitime est l'observation : voir le débit s'établir, le taux
// d'allocation se stabiliser et l'avantage de la maison converger pendant que
// la simulation tourne.
type Runner struct {
	shoe  *Shoe
	rules Rules
	bet   float64
	side  SideBets
	stats Stats
}

// NewRunner ouvre un dérouleur. Le générateur est initialisé par seed, donc
// deux dérouleurs de même graine et de même découpage en lots produisent les
// mêmes cartes.
func NewRunner(seed int64, r Rules, bet float64, sb SideBets) *Runner {
	rng := rand.New(rand.NewSource(seed))
	return &Runner{
		shoe:  NewShoe(r.NumDecks, r.Penetration, rng),
		rules: r,
		bet:   bet,
		side:  sb,
	}
}

// RunBatch joue n coups de plus et les agrège aux statistiques courantes.
//
// Le découpage en lots ne change pas la suite de cartes : le sabot et son
// générateur survivent d'un lot au suivant. Les statistiques finales sont donc
// identiques à celles d'une exécution d'un seul bloc de même graine — seul le
// temps mesuré diffère.
func (rn *Runner) RunBatch(n int) {
	for i := 0; i < n; i++ {
		// Le sabot se rebat entre deux coups, jamais au milieu d'un coup.
		if rn.shoe.CutReached() {
			rn.shoe.Shuffle()
		}
		rn.stats.Add(PlayRound(rn.shoe, rn.rules, rn.bet, rn.side))
	}
	rn.stats.Shuffles = rn.shoe.Shuffles
	rn.stats.CardsDealt = rn.shoe.CardsDealt
}

// Stats renvoie les statistiques cumulées depuis l'ouverture du dérouleur.
func (rn *Runner) Stats() Stats { return rn.stats }
