package blackjack

// RoundResult agrège le résultat d'un coup.
//
// MainWagered ne retient que la mise initiale : c'est le dénominateur de
// l'avantage de la maison au sens usuel. Action retient en revanche tout ce
// qui a été réellement engagé, splits et doubles compris, et sert à calculer
// l'« element of risk ». Distinguer les deux est indispensable pour que les
// chiffres du rapport soient comparables aux valeurs publiées.
type RoundResult struct {
	MainWagered float64
	Action      float64
	MainNet     float64
	SideWagered float64
	SideNet     float64
	Hands       int
	PlayerBJ    bool
	DealerBJ    bool
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
func PlayRound(s *Shoe, r Rules, bet float64, sb SideBets) RoundResult {
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

	dealer := &Hand{Cards: []Card{up, hole}}
	hands := []*Hand{{Cards: []Card{p1, p2}, Bet: bet}}

	dealerBJ := dealer.IsBlackjack()
	playerBJ := hands[0].IsBlackjack()
	res.DealerBJ = dealerBJ
	res.PlayerBJ = playerBJ

	// --- Paris annexes jugés sur la distribution initiale ---
	if sb.PerfectPairs > 0 {
		m, _ := EvalPerfectPairs(p1, p2)
		res.SideNet += netSide(sb.PerfectPairs, m)
	}
	if sb.TwentyOnePlus3 > 0 {
		m, _ := EvalTwentyOnePlus3(p1, p2, up)
		res.SideNet += netSide(sb.TwentyOnePlus3, m)
	}
	if sb.LuckyLadies > 0 {
		m, _ := EvalLuckyLadies(p1, p2, dealerBJ)
		res.SideNet += netSide(sb.LuckyLadies, m)
	}

	// --- Assurance ---
	// Proposée uniquement sur un As visible, et refusée par la stratégie de
	// base. Le code est présent pour que la règle soit complète et mesurable.
	if r.OfferInsurance && up.IsAce() && TakeInsurance() {
		ins := bet / 2
		if dealerBJ {
			res.MainNet += ins * 2
		} else {
			res.MainNet -= ins
		}
	}

	// --- Contrôle de la carte cachée ---
	if dealerBJ {
		if !playerBJ {
			res.MainNet -= bet
		}
		// Un blackjack des deux côtés est une égalité : rien n'est échangé.
		res.Hands = 1
		if sb.Buster > 0 {
			// Le croupier ne tire pas, donc il ne saute pas.
			res.SideNet += netSide(sb.Buster, 0)
		}
		return res
	}

	// --- Blackjack du joueur ---
	if playerBJ {
		res.MainNet += bet * r.BlackjackPayout
		res.Hands = 1
		if sb.Buster > 0 {
			// Le croupier complète sa main pour que le Buster soit jugeable.
			playDealer(dealer, s, r)
			m, _ := EvalBuster(len(dealer.Cards), dealer.IsBust())
			res.SideNet += netSide(sb.Buster, m)
			res.DealerPlayed = true
			res.DealerBust = dealer.IsBust()
		}
		return res
	}

	// --- Décisions du joueur ---
	// La boucle parcourt un slice qui grandit : chaque split y ajoute une
	// main, qui sera jouée à son tour.
	for i := 0; i < len(hands); i++ {
		h := hands[i]
		for {
			if h.Surrendered || h.Stood || h.IsBust() {
				break
			}
			// Un As séparé ne reçoit qu'une seule carte, sauf règle contraire.
			if h.SplitAce && !r.HitSplitAces && len(h.Cards) >= 2 {
				h.Stood = true
				break
			}

			switch Decide(h, up, r, len(hands)) {
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
				// La seconde carte part fonder une nouvelle main, puis chaque
				// moitié reçoit une carte.
				second := h.Cards[1]
				nh := &Hand{
					Cards:     []Card{second},
					Bet:       bet,
					FromSplit: true,
					SplitAce:  second.IsAce(),
				}
				h.Cards = []Card{h.Cards[0]}
				h.FromSplit = true
				h.SplitAce = h.Cards[0].IsAce()
				h.Add(s.Deal())
				nh.Add(s.Deal())
				hands = append(hands, nh)
				res.Action += bet
			}
		}
	}
	res.Hands = len(hands)

	// --- Main du croupier ---
	// Le croupier ne tire que s'il reste une main à battre. Il complète
	// néanmoins sa main quand un pari Buster est en jeu, puisque celui-ci
	// porte précisément sur son dépassement.
	live := false
	for _, h := range hands {
		if !h.Surrendered && !h.IsBust() {
			live = true
			break
		}
	}
	if live || sb.Buster > 0 {
		playDealer(dealer, s, r)
		res.DealerPlayed = true
	}

	dealerTotal, _ := dealer.Total()
	dealerBust := dealer.IsBust()
	res.DealerBust = dealerBust

	// --- Règlements ---
	for _, h := range hands {
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
		m, _ := EvalBuster(len(dealer.Cards), dealerBust)
		res.SideNet += netSide(sb.Buster, m)
	}

	return res
}

// playDealer complète la main du croupier : il tire jusqu'à 17, et sur un 17
// souple selon la règle de la table (H17 ou S17).
func playDealer(d *Hand, s *Shoe, r Rules) {
	for {
		t, soft := d.Total()
		if t < 17 || (t == 17 && soft && r.DealerHitsSoft17) {
			d.Add(s.Deal())
			continue
		}
		return
	}
}
