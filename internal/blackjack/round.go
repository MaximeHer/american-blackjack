package blackjack

// RoundResult agrège le résultat d'un coup.
//
// MainWagered ne retient que la mise initiale : c'est le dénominateur de
// l'avantage de la maison au sens usuel. Action retient en revanche tout ce
// qui a été réellement engagé, splits et doubles compris, et sert à calculer
// l'« element of risk ». Distinguer les deux est indispensable pour que les
// chiffres du rapport soient comparables aux valeurs publiées.
//
// Champs ordonnés par taille décroissante : 80 octets pour 52 utiles avant
// réordonnancement, soit 28 octets de remplissage (35 %).
type RoundResult struct {
	MainWagered float64
	Action      float64
	MainNet     float64
	SideWagered float64
	SideNet     float64
	Hands       int

	PlayerBJ bool
	DealerBJ bool
	// DealerPlayed indique que le croupier a effectivement complété sa main.
	// Il ne le fait pas quand le coup est déjà résolu — blackjack de part et
	// d'autre, ou toutes les mains du joueur sautées. Les fréquences de
	// dépassement du croupier doivent être rapportées à ce compteur, et non
	// au nombre total de coups, sous peine d'être sous-estimées.
	DealerPlayed bool
	DealerBust   bool
}

// PlayRound joue un coup complet de blackjack américain :
//
//  1. les paris annexes sont engagés ;
//  2. la distribution donne deux cartes au joueur, une carte visible et une
//     carte cachée au croupier ;
//  3. les paris annexes résolus sur la distribution initiale sont réglés ;
//  4. l'assurance est proposée si la carte visible est un As ;
//  5. le croupier contrôle sa carte cachée (peek) si sa carte visible est un
//     As ou une bûche ; un blackjack du croupier termine immédiatement le coup ;
//  6. le joueur joue ses mains, splits et doubles compris ;
//  7. le croupier complète sa main ;
//  8. les règlements sont effectués, puis le pari Buster est résolu.
func PlayRound(s *Shoe, r Rules, bet float64, sb SideBets, tr *Trace) RoundResult {
	res := RoundResult{
		MainWagered: bet,
		Action:      bet,
		SideWagered: sb.Total(),
	}

	// --- Distribution initiale, dans l'ordre réel de la table ---
	p1 := s.Deal()
	up := s.Deal()
	p2 := s.Deal()
	hole := s.Deal()

	// Mains en variables LOCALES, donc sur la pile. Un tableau de 4 mains
	// occupe 160 octets que le compilateur met à zéro d'un seul memclr, là où
	// la version précédente faisait quatre allocations sur le tas par coup.
	var hands [maxHands]Hand
	var dealer Hand
	nHands := 1

	dealer.Add(up)
	dealer.Add(hole)
	hands[0].Bet = bet
	hands[0].Add(p1)
	hands[0].Add(p2)

	if tr != nil {
		tr.add("Mise de %.2f sur la case principale.", bet)
		tr.add("Joueur : %s et %s.", p1.Label(), p2.Label())
		tr.add("Croupier : %s visible, une carte cachée.", up.Label())
	}

	dealerBJ := dealer.IsBlackjack()
	playerBJ := hands[0].IsBlackjack()
	res.DealerBJ = dealerBJ
	res.PlayerBJ = playerBJ

	// --- Paris annexes jugés sur la distribution initiale ---
	if sb.PerfectPairs > 0 {
		o := EvalPerfectPairs(p1, p2)
		res.SideNet += netSide(sb.PerfectPairs, o)
		if tr != nil {
			tr.add("Perfect Pairs : %s.", o.Label())
		}
	}
	if sb.TwentyOnePlus3 > 0 {
		o := EvalTwentyOnePlus3(p1, p2, up)
		res.SideNet += netSide(sb.TwentyOnePlus3, o)
		if tr != nil {
			tr.add("21+3 : %s.", o.Label())
		}
	}
	if sb.LuckyLadies > 0 {
		o := EvalLuckyLadies(p1, p2, dealerBJ)
		res.SideNet += netSide(sb.LuckyLadies, o)
		if tr != nil {
			tr.add("Lucky Ladies : %s.", o.Label())
		}
	}

	// --- Assurance ---
	// Proposée uniquement sur un As visible, et refusée par la stratégie de
	// base. Le code est présent pour que la règle soit complète et mesurable.
	if r.OfferInsurance && up.IsAce() && TakeInsurance(up) {
		ins := bet / 2
		if dealerBJ {
			res.MainNet += ins * 2
		} else {
			res.MainNet -= ins
		}
		if tr != nil {
			tr.add("Assurance prise pour %.2f.", ins)
		}
	}

	// --- Contrôle de la carte cachée ---
	if dealerBJ {
		if tr != nil {
			tr.add("Le croupier retourne %s : blackjack.", hole.Label())
		}
		if !playerBJ {
			res.MainNet -= bet
		}
		// Un blackjack des deux côtés est une égalité : rien n'est échangé.
		res.Hands = 1
		if sb.Buster > 0 {
			// Le croupier ne tire pas, donc il ne saute pas.
			res.SideNet += netSide(sb.Buster, OutcomeLose)
		}
		return res
	}

	// --- Blackjack du joueur ---
	if playerBJ {
		res.MainNet += bet * r.BlackjackPayout
		res.Hands = 1
		if tr != nil {
			tr.add("Blackjack du joueur, payé %.2f pour 1.", r.BlackjackPayout)
		}
		if sb.Buster > 0 {
			// Le croupier complète sa main pour que le Buster soit jugeable.
			playDealer(&dealer, s, r, tr)
			res.SideNet += netSide(sb.Buster, EvalBuster(dealer.Len(), dealer.IsBust()))
			res.DealerPlayed = true
			res.DealerBust = dealer.IsBust()
		}
		return res
	}

	// --- Décisions du joueur ---
	// La boucle parcourt un slice qui grandit : chaque split y ajoute une
	// main, qui sera jouée à son tour.
	for i := 0; i < nHands; i++ {
		h := &hands[i]
		for {
			if h.Surrendered || h.Stood || h.IsBust() {
				break
			}
			// Un As séparé ne reçoit qu'une seule carte, sauf règle contraire.
			if h.SplitAce && !r.HitSplitAces && h.Len() >= 2 {
				h.Stood = true
				break
			}

			action := decideBasic(h, up, r, nHands)
			if tr != nil {
				tr.add("Main %d (%s) : %s.", i+1, h.Describe(), actionLabel(action))
			}

			switch action {
			case Stand:
				h.Stood = true

			case Hit:
				h.Add(s.Deal())

			case Double:
				res.Action += h.Bet
				h.Bet *= 2
				h.Doubled = true
				h.Add(s.Deal())
				h.Stood = true

			case Surrender:
				h.Surrendered = true

			case Split:
				// La seconde carte part fonder une nouvelle main dans
				// l'emplacement libre suivant du tableau. Aucune allocation :
				// les quatre emplacements existent déjà sur la pile.
				//
				// Le garde-fou nHands < maxHands double celui de la stratégie :
				// une Rules mal configurée ne doit pas pouvoir déborder du
				// tableau.
				if nHands >= maxHands {
					h.Stood = true
					break
				}
				second := h.Card(1)

				h.keepFirst()
				h.FromSplit = true
				h.SplitAce = h.Card(0).IsAce()
				h.Add(s.Deal())

				nh := &hands[nHands]
				nh.Bet = bet
				nh.FromSplit = true
				nh.SplitAce = second.IsAce()
				nh.Add(second)
				nh.Add(s.Deal())
				nHands++
				res.Action += bet
			}
		}
	}
	res.Hands = nHands

	// --- Main du croupier ---
	// Le croupier ne tire que s'il reste une main à battre. Il complète
	// néanmoins sa main quand un pari Buster est en jeu, puisque celui-ci
	// porte précisément sur son dépassement.
	live := false
	for i := 0; i < nHands; i++ {
		if h := &hands[i]; !h.Surrendered && !h.IsBust() {
			live = true
			break
		}
	}
	if live || sb.Buster > 0 {
		if tr != nil {
			tr.add("Le croupier retourne %s.", hole.Label())
		}
		playDealer(&dealer, s, r, tr)
		res.DealerPlayed = true
	}

	dealerTotal, _ := dealer.Total()
	dealerBust := dealer.IsBust()
	res.DealerBust = dealerBust

	// --- Règlements ---
	for i := 0; i < nHands; i++ {
		h := &hands[i]
		switch {
		case h.Surrendered:
			res.MainNet -= h.Bet / 2
		case h.IsBust():
			res.MainNet -= h.Bet
		case dealerBust:
			res.MainNet += h.Bet
		default:
			t, _ := h.Total()
			if t > dealerTotal {
				res.MainNet += h.Bet
			} else if t < dealerTotal {
				res.MainNet -= h.Bet
			}
			// Totaux égaux : égalité, rien n'est échangé.
		}
	}

	// --- Pari Buster ---
	if sb.Buster > 0 {
		o := EvalBuster(dealer.Len(), dealerBust)
		res.SideNet += netSide(sb.Buster, o)
		if tr != nil {
			tr.add("Buster Blackjack : %s.", o.Label())
		}
	}

	if tr != nil {
		tr.add("Résultat net du coup : %+.2f.", res.MainNet+res.SideNet)
	}
	return res
}

// playDealer complète la main du croupier : il tire jusqu'à 17, et sur un 17
// souple selon la règle de la table (H17 ou S17).
//
// tr peut être nil quand la narration n'est pas souhaitée.
func playDealer(d *Hand, s *Shoe, r Rules, tr *Trace) {
	for {
		t, soft := d.Total()
		if t < 17 || (t == 17 && soft && r.DealerHitsSoft17) {
			c := s.Deal()
			d.Add(c)
			if tr != nil {
				tr.add("Le croupier tire %s.", c.Label())
			}
			continue
		}
		return
	}
}
