//go:build !instrument

package blackjack

// Compilation PAR DÉFAUT : aucun compteur d'opérations.
//
// Instrumented est une constante fausse, donc tout bloc `if Instrumented { … }`
// est éliminé à la compilation, argument compris. Le binaire produit ne
// contient pas une seule instruction de comptage : la boucle mesurée est
// exactement celle qui tournerait en production.
//
// C'est le point essentiel du dispositif. Un compteur, même réduit à une
// incrémentation, modifierait le code mesuré — et l'on ne mesurerait plus le
// moteur mais le moteur plus son instrumentation.
//
// Pour obtenir les comptages, recompiler avec :
//
//	go build -tags instrument ./cmd/simulate
//	go test -tags instrument -run TestOpCounts -v ./internal/blackjack
const Instrumented = false

// OpCounts rapporte le nombre d'opérations élémentaires exécutées.
type OpCounts struct {
	Enabled      bool   `json:"enabled"`
	Decisions    uint64 `json:"decisions"`
	HandTotals   uint64 `json:"handTotals"`
	CardValues   uint64 `json:"cardValues"`
	ShuffleMoves uint64 `json:"shuffleMoves"`
	MapLookups   uint64 `json:"mapLookups"`
}

// Ops renvoie des compteurs vides : le binaire n'est pas instrumenté.
func Ops() OpCounts { return OpCounts{} }

// ResetOps n'a aucun effet dans un binaire non instrumenté.
func ResetOps() {}

func countDecide()          {}
func countHandTotal()       {}
func countCardValue()       {}
func countMapLookup()       {}
func countShuffleMoves(int) {}
