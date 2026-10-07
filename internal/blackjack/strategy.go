package blackjack

// Décisions possibles du joueur.
const (
	Hit       = "H"
	Stand     = "S"
	Double    = "D"
	Split     = "P"
	Surrender = "R"
)

// Strategy décide du coup à jouer. C'est le point d'extension du moteur : une
// stratégie avec comptage de cartes viendrait s'y brancher.
//
// PALIER 3. Le moteur NE PASSE PAS par cette interface. La version de référence
// routait chaque décision à travers elle, ce qui est le réflexe orienté objet et
// se paie : l'appel devient dynamique, donc le compilateur ne peut ni l'inliner
// ni spécialiser son corps, alors qu'il n'existe qu'une seule implémentation
// connue à la compilation.
//
// L'interface est conservée pour ce qu'elle apporte réellement — la possibilité
// de substituer une autre stratégie — mais le chemin chaud appelle directement
// decideBasic. Les deux besoins sont ainsi satisfaits sans que l'un ne grève
// l'autre.
type Strategy interface {
	Decide(h *Hand, up Card, r Rules, handCount int) string
	TakeInsurance(up Card) bool
	Name() string
}

// BasicStrategy expose la stratégie de base derrière l'interface Strategy, pour
// les appelants qui veulent la manipuler comme telle. Ses méthodes délèguent aux
// fonctions libres, qui sont ce que le moteur appelle.
type BasicStrategy struct{}

// Name identifie la stratégie dans le journal d'un coup.
func (b *BasicStrategy) Name() string { return "stratégie de base" }

// Decide satisfait l'interface Strategy en déléguant à la fonction libre.
func (b *BasicStrategy) Decide(h *Hand, up Card, r Rules, handCount int) string {
	return decideBasic(h, up, r, handCount)
}

// TakeInsurance satisfait l'interface Strategy en déléguant à la fonction libre.
func (b *BasicStrategy) TakeInsurance(up Card) bool { return takeInsuranceBasic(up) }

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

// Codes de décision internes. dNone vaut zéro, donc une case non renseignée de
// la table plate est naturellement « pas de règle », sans initialisation
// explicite.
const (
	dNone uint8 = iota
	dHit
	dStand
	dDouble
	dSplit
	dSurrender
)

// decisionNames traduit un code en décision publique. Les chaînes sont des
// constantes statiques : les retourner ne coûte aucune allocation.
var decisionNames = [...]string{
	dNone:      "",
	dHit:       Hit,
	dStand:     Stand,
	dDouble:    Double,
	dSplit:     Split,
	dSurrender: Surrender,
}

// Dimensions de la table plate.
const (
	kindHard  = 0
	kindSoft  = 1
	kindPair  = 2
	kindCount = 3
	maxTotal  = 22 // totaux de 0 à 21 ; les paires sont indexées par leur valeur, 2 à 11
	upCount   = 10
)

// strategyFlat est la stratégie de base sous forme de TABLEAU PLAT, indexé
// arithmétiquement.
//
// RANG 4 DU PROFIL. La version précédente construisait une clé textuelle par
// fmt.Sprintf — "hard-16-10" — puis hachait cette chaîne pour interroger une
// map. Le profil attribuait 2,64 s à cette seule ligne, soit 35 % du temps
// total du programme et 87 % du coût de decideBasic.
//
// L'état de décision est pourtant entièrement décrit par trois entiers bornés :
// le type de main (3 valeurs), le total (5 à 21) et la carte visible du
// croupier (10 valeurs). Un tableau de 3 x 22 x 10 cases d'un octet pèse
// 660 octets — soit une dizaine de lignes de cache — et son accès se réduit à
// deux multiplications et une addition, sans formatage, sans allocation et sans
// hachage.
//
// C'est l'application directe du compromis espace-temps du cours : remplacer un
// calcul par une lecture indexée directe.
var strategyFlat [kindCount][maxTotal][upCount]uint8

// upIdxByRank convertit le rang d'une carte visible en colonne de la table :
// les quatre rangs de valeur 10 partagent la colonne 8, l'As occupe la 9.
var upIdxByRank = [rankCount]uint8{
	rank2: 0, rank3: 1, rank4: 2, rank5: 3, rank6: 4,
	rank7: 5, rank8: 6, rank9: 7,
	rank10: 8, rankJack: 8, rankQueen: 8, rankKing: 8,
	rankAce: 9,
}

// decisionCode traduit le caractère d'une ligne source en code interne.
func decisionCode(ch byte) uint8 {
	switch ch {
	case 'H':
		return dHit
	case 'S':
		return dStand
	case 'D':
		return dDouble
	case 'P':
		return dSplit
	case 'R':
		return dSurrender
	}
	return dNone
}

// init remplit la table plate à partir des MÊMES lignes sources que celles
// exposées par ExportStrategy.
//
// C'est une précaution délibérée : recopier la table à la main serait la façon
// la plus sûre d'introduire une erreur silencieuse de stratégie, qui dégraderait
// l'avantage de la maison sans provoquer la moindre erreur visible.
// TestStrategyFlatMatchesRows vérifie cette correspondance case par case.
func init() {
	for total, row := range hardRows {
		for i := 0; i < upCount; i++ {
			strategyFlat[kindHard][total][i] = decisionCode(row[i])
		}
	}
	for total, row := range softRows {
		for i := 0; i < upCount; i++ {
			strategyFlat[kindSoft][total][i] = decisionCode(row[i])
		}
	}
	for rank, row := range pairRows {
		v := pairValue(rank)
		for i := 0; i < upCount; i++ {
			strategyFlat[kindPair][v][i] = decisionCode(row[i])
		}
	}
}

// pairValue convertit le rang normalisé d'une paire en sa valeur au blackjack,
// qui sert d'index : 11 pour les As, 10 pour les bûches, le rang lui-même sinon.
func pairValue(rank string) int {
	switch rank {
	case "A":
		return 11
	case "10":
		return 10
	}
	return int(rank[0] - '0')
}

// Decide applique la stratégie de base à une main, puis dégrade la décision si
// la règle de la table ne permet pas de la jouer : double interdit, splits
// épuisés, abandon non proposé.
//
// handCount est le nombre de mains déjà en jeu pour ce coup, nécessaire pour
// savoir si un split supplémentaire est encore autorisé.
func decideBasic(h *Hand, up Card, r Rules, handCount int) string {
	countDecide()
	ui := int(upIdxByRank[up.rank()])
	total, soft := h.Total()

	// Une paire se consulte d'abord dans sa propre table, et seulement si on
	// peut encore ouvrir une main supplémentaire.
	if h.IsPair() && handCount < r.MaxSplitHands {
		if d := strategyFlat[kindPair][h.Cards[0].Value()][ui]; d != dNone {
			if d == dSplit {
				if canSplit(h, r) {
					return Split
				}
			} else {
				return degrade(d, h, r, total, soft)
			}
		}
	}

	kind := kindHard
	if soft {
		kind = kindSoft
	}

	var d uint8
	if total < maxTotal {
		d = strategyFlat[kind][total][ui]
	}
	if d == dNone {
		// Totaux souples inférieurs à 13 (une paire d'As non séparable) :
		// aucun risque de dépasser, on tire.
		if total >= 17 {
			return Stand
		}
		return Hit
	}
	return degrade(d, h, r, total, soft)
}

// takeInsuranceBasic décide de prendre ou non l'assurance. La stratégie de base
// la refuse toujours : sans comptage de cartes, c'est un pari dont l'espérance
// est négative d'environ 7 %.
func takeInsuranceBasic(up Card) bool { return false }

// Decide applique la stratégie de base. Appel statique, inlinable, sans
// indirection par table de méthodes.
func Decide(h *Hand, up Card, r Rules, handCount int) string {
	return decideBasic(h, up, r, handCount)
}

// TakeInsurance applique la stratégie de base.
func TakeInsurance(up Card) bool { return takeInsuranceBasic(up) }

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
func degrade(d uint8, h *Hand, r Rules, total int, soft bool) string {
	switch d {
	case dDouble:
		if canDouble(h, r) {
			return Double
		}
		if soft && total >= 18 {
			return Stand
		}
		return Hit
	case dSurrender:
		if r.LateSurrender && len(h.Cards) == 2 && !h.FromSplit {
			return Surrender
		}
		return Hit
	}
	return decisionNames[d]
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
