package blackjack

// Rules rassemble les variantes de règles d'une table. Toutes sont
// paramétrables : mesurer l'impact de chaque variante sur l'avantage de la
// maison est l'un des usages du moteur.
// Rules est passée PAR VALEUR à PlayRound, donc recopiée à chaque coup : son
// gaspillage de remplissage se paie à chaque appel. Champs ordonnés par taille
// décroissante — 56 octets pour 39 utiles avant réordonnancement.
type Rules struct {
	// Penetration est la fraction du sabot distribuée avant rebattage.
	Penetration float64
	// BlackjackPayout est le multiplicateur du gain sur blackjack : 1.5 pour
	// le classique 3:2, 1.2 pour le 6:5 défavorable.
	BlackjackPayout float64
	// NumDecks est le nombre de jeux dans le sabot.
	NumDecks int
	// MaxSplitHands est le nombre maximal de mains simultanées après splits.
	MaxSplitHands int

	// DealerHitsSoft17 : le croupier tire sur 17 souple (H17) au lieu de
	// rester (S17). La règle H17 augmente l'avantage de la maison d'environ
	// 0,20 point.
	DealerHitsSoft17 bool
	// DoubleAnyTwo autorise le double sur n'importe quelles deux cartes.
	// Si faux, le double est restreint aux totaux de 9 à 11.
	DoubleAnyTwo bool
	// DoubleAfterSplit autorise le double sur une main issue d'un split (DAS).
	DoubleAfterSplit bool
	// ResplitAces autorise de reséparer une paire d'As déjà séparée.
	ResplitAces bool
	// HitSplitAces autorise de tirer plus d'une carte sur un As séparé.
	HitSplitAces bool
	// LateSurrender autorise l'abandon tardif, après le contrôle de la carte
	// cachée du croupier, contre la moitié de la mise.
	LateSurrender bool
	// OfferInsurance propose l'assurance quand la carte visible est un As.
	OfferInsurance bool
}

// DefaultRules renvoie la configuration de référence du projet : blackjack
// américain à 4 jeux, croupier restant sur 17 souple, blackjack payé 3:2,
// double autorisé partout y compris après split, abandon tardif autorisé.
//
// Ces règles donnent un avantage de la maison attendu d'environ 0,44 % contre
// une stratégie de base. C'est cette valeur publiée qui sert d'oracle de
// non-régression : toute optimisation doit la laisser inchangée.
func DefaultRules() Rules {
	return Rules{
		NumDecks:         4,
		Penetration:      0.75,
		DealerHitsSoft17: false,
		BlackjackPayout:  1.5,
		DoubleAnyTwo:     true,
		DoubleAfterSplit: true,
		MaxSplitHands:    4,
		ResplitAces:      false,
		HitSplitAces:     false,
		LateSurrender:    true,
		OfferInsurance:   true,
	}
}
