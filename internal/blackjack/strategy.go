package blackjack

import "fmt"

// Décisions possibles du joueur.
const (
	Hit       = "H"
	Stand     = "S"
	Double    = "D"
	Split     = "P"
	Surrender = "R"
)

// Strategy décide du coup à jouer.
//
// VERSION DE RÉFÉRENCE : faire passer la décision par une interface est le
// réflexe orienté objet, et il se paie. L'appel devient dynamique — une
// indirection par table de méthodes à chaque décision — ce qui empêche le
// compilateur d'inliner Decide et donc de propager les constantes ou
// d'éliminer les branches mortes. Le moteur n'a pourtant qu'une seule
// implémentation, connue à la compilation.
type Strategy interface {
	Decide(h *Hand, up *Card, r Rules, handCount int) string
	TakeInsurance(up *Card) bool
	Name() string
}

// DefaultStrategy est la stratégie appliquée par le moteur.
var DefaultStrategy Strategy = &BasicStrategy{}

// BasicStrategy applique la stratégie de base publiée.
type BasicStrategy struct{}

// Name identifie la stratégie dans le journal d'un coup.
func (b *BasicStrategy) Name() string { return "stratégie de base" }

// dealerColumns donne l'ordre des colonnes dans les tables ci-dessous :
// la carte visible du croupier, de 2 à l'As.
var dealerColumns = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "A"}

// Stratégie de base pour 4 à 8 jeux, croupier restant sur 17 souple (S17),
// double autorisé après split, abandon tardif autorisé.
//
// Chaque ligne est indexée par le total du joueur et contient les 10 décisions
// face aux 10 cartes visibles possibles du croupier.
var hardRows = map[int]string{
	5:  "HHHHHHHHHH",
	6:  "HHHHHHHHHH",
	7:  "HHHHHHHHHH",
	8:  "HHHHHHHHHH",
	9:  "HDDDDHHHHH",
	10: "DDDDDDDDHH",
	11: "DDDDDDDDDH",
	12: "HHSSSHHHHH",
	13: "SSSSSHHHHH",
	14: "SSSSSHHHHH",
	15: "SSSSSHHHRH",
	16: "SSSSSHHRRH",
	17: "SSSSSSSSSS",
	18: "SSSSSSSSSS",
	19: "SSSSSSSSSS",
	20: "SSSSSSSSSS",
	21: "SSSSSSSSSS",
}

// Mains souples, indexées par leur total As compté 11 : 13 vaut As-2,
// 18 vaut As-7, etc.
var softRows = map[int]string{
	13: "HHHDDHHHHH",
	14: "HHHDDHHHHH",
	15: "HHDDDHHHHH",
	16: "HHDDDHHHHH",
	17: "HDDDDHHHHH",
	18: "SDDDDSSHHH",
	19: "SSSSSSSSSS",
	20: "SSSSSSSSSS",
	21: "SSSSSSSSSS",
}

// Paires, indexées par le rang normalisé de la carte dédoublée.
var pairRows = map[string]string{
	"A":  "PPPPPPPPPP",
	"10": "SSSSSSSSSS",
	"9":  "PPPPPSPPSS",
	"8":  "PPPPPPPPPP",
	"7":  "PPPPPPHHHH",
	"6":  "PPPPPHHHHH",
	"5":  "DDDDDDDDHH",
	"4":  "HHHPPHHHHH",
	"3":  "PPPPPPHHHH",
	"2":  "PPPPPPHHHH",
}

// strategyTable est construite au démarrage, avec des clés textuelles de la
// forme "hard-16-10", "soft-18-3" ou "pair-8-A".
//
// VERSION DE RÉFÉRENCE. C'est l'un des hot paths les plus coûteux du moteur :
// chaque décision construit sa clé par fmt.Sprintf — donc un formatage et une
// allocation — puis hache la chaîne obtenue pour interroger une map. Le palier
// d'optimisation correspondant remplacera tout cela par une indexation
// arithmétique dans un tableau plat, sans allocation ni hachage.
var strategyTable = map[string]string{}

func init() {
	for total, row := range hardRows {
		for i, col := range dealerColumns {
			strategyTable[fmt.Sprintf("hard-%d-%s", total, col)] = string(row[i])
		}
	}
	for total, row := range softRows {
		for i, col := range dealerColumns {
			strategyTable[fmt.Sprintf("soft-%d-%s", total, col)] = string(row[i])
		}
	}
	for rank, row := range pairRows {
		for i, col := range dealerColumns {
			strategyTable[fmt.Sprintf("pair-%s-%s", rank, col)] = string(row[i])
		}
	}
}

// Decide applique la stratégie de base à une main, puis dégrade la décision si
// la règle de la table ne permet pas de la jouer : double interdit, splits
// épuisés, abandon non proposé.
//
// handCount est le nombre de mains déjà en jeu pour ce coup, nécessaire pour
// savoir si un split supplémentaire est encore autorisé.
func (b *BasicStrategy) Decide(h *Hand, up *Card, r Rules, handCount int) string {
	countDecide()
	upKey := up.NormalizedRank()
	total, soft := h.Total()

	// Une paire se consulte d'abord dans sa propre table, et seulement si on
	// peut encore ouvrir une main supplémentaire.
	if h.IsPair() && handCount < r.MaxSplitHands {
		rank := h.Cards[0].NormalizedRank()
		countMapLookup()
		if d, ok := strategyTable[fmt.Sprintf("pair-%s-%s", rank, upKey)]; ok {
			if d == Split {
				if canSplit(h, r) {
					return Split
				}
			} else {
				return degrade(d, h, r, total, soft)
			}
		}
	}

	kind := "hard"
	if soft {
		kind = "soft"
	}
	countMapLookup()
	d, ok := strategyTable[fmt.Sprintf("%s-%d-%s", kind, total, upKey)]
	if !ok {
		// Totaux souples inférieurs à 13 (une paire d'As non séparable) :
		// aucun risque de dépasser, on tire.
		if total >= 17 {
			return Stand
		}
		return Hit
	}
	return degrade(d, h, r, total, soft)
}

// TakeInsurance décide de prendre ou non l'assurance. La stratégie de base la
// refuse toujours : sans comptage de cartes, c'est un pari dont l'espérance
// est négative d'environ 7 %.
func (b *BasicStrategy) TakeInsurance(up *Card) bool { return false }

// Decide applique la stratégie par défaut du moteur. Conservé comme fonction
// libre pour les appelants qui n'ont pas à connaître l'interface.
func Decide(h *Hand, up *Card, r Rules, handCount int) string {
	return DefaultStrategy.Decide(h, up, r, handCount)
}

// TakeInsurance applique la stratégie par défaut du moteur.
func TakeInsurance(up *Card) bool { return DefaultStrategy.TakeInsurance(up) }

// canSplit vérifie qu'une paire est effectivement séparable compte tenu des
// règles : une paire d'As déjà issue d'un split ne se resépare que si la table
// l'autorise.
func canSplit(h *Hand, r Rules) bool {
	if len(h.Cards) != 2 {
		return false
	}
	if h.Cards[0].IsAce() && h.FromSplit && !r.ResplitAces {
		return false
	}
	return true
}

// degrade remplace une décision impossible par la meilleure décision
// autorisée. Un double interdit devient un tirage, sauf sur une main souple
// de 18 ou plus où il faut rester. Un abandon interdit devient un tirage.
func degrade(d string, h *Hand, r Rules, total int, soft bool) string {
	switch d {
	case Double:
		if canDouble(h, r) {
			return Double
		}
		if soft && total >= 18 {
			return Stand
		}
		return Hit
	case Surrender:
		if r.LateSurrender && len(h.Cards) == 2 && !h.FromSplit {
			return Surrender
		}
		return Hit
	}
	return d
}

// canDouble vérifie qu'un double est autorisé sur cette main.
func canDouble(h *Hand, r Rules) bool {
	if len(h.Cards) != 2 {
		return false
	}
	if h.FromSplit && !r.DoubleAfterSplit {
		return false
	}
	if !r.DoubleAnyTwo {
		t, _ := h.Total()
		return t >= 9 && t <= 11
	}
	return true
}

// actionLabel traduit une décision pour le journal narratif d'un coup.
//
// VERSION DE RÉFÉRENCE : appelée à chaque décision, y compris en simulation.
func actionLabel(d string) string {
	switch d {
	case Hit:
		return "tire"
	case Stand:
		return "reste"
	case Double:
		return "double"
	case Split:
		return "sépare"
	case Surrender:
		return "abandonne"
	}
	return d
}
