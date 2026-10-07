//go:build instrument

package blackjack

import "sync/atomic"

// Compilation INSTRUMENTÉE, activée par `-tags instrument`.
//
// Ce binaire compte les opérations élémentaires du moteur. Il ne doit jamais
// servir à mesurer un débit, seulement à expliquer d'où vient le travail.
//
// Mesure honnête du surcoût, à la vitesse de la version de référence : il est
// NOYÉ DANS LE BRUIT (334 522 contre 339 422 coups/s, soit un écart inférieur
// à la dispersion des mesures). Le moteur est si lent que les compteurs
// atomiques ne pèsent rien devant ses allocations. Cela cessera d'être vrai à
// mesure qu'il sera optimisé, et le surcoût devra être remesuré à chaque
// palier plutôt que supposé constant.
//
// L'intérêt est de transformer des affirmations en mesures. « Le mélange est
// quadratique » devient « 10 756 déplacements d'éléments par rebattage, contre
// 208 pour Fisher-Yates ». « Total() est appelé plusieurs fois par décision »
// devient « 7,81 fois par décision ».
const Instrumented = true

// OpCounts rapporte le nombre d'opérations élémentaires exécutées.
type OpCounts struct {
	Enabled      bool   `json:"enabled"`
	Decisions    uint64 `json:"decisions"`
	HandTotals   uint64 `json:"handTotals"`
	CardValues   uint64 `json:"cardValues"`
	ShuffleMoves uint64 `json:"shuffleMoves"`
	MapLookups   uint64 `json:"mapLookups"`
}

// Compteurs atomiques : le moteur deviendra concurrent en phase C, et un
// comptage faux serait pire qu'absent.
var (
	cDecisions    atomic.Uint64
	cHandTotals   atomic.Uint64
	cCardValues   atomic.Uint64
	cShuffleMoves atomic.Uint64
	cMapLookups   atomic.Uint64
)

// Ops renvoie l'état courant des compteurs.
func Ops() OpCounts {
	return OpCounts{
		Enabled:      true,
		Decisions:    cDecisions.Load(),
		HandTotals:   cHandTotals.Load(),
		CardValues:   cCardValues.Load(),
		ShuffleMoves: cShuffleMoves.Load(),
		MapLookups:   cMapLookups.Load(),
	}
}

// ResetOps remet les compteurs à zéro, afin d'isoler une phase de mesure.
func ResetOps() {
	cDecisions.Store(0)
	cHandTotals.Store(0)
	cCardValues.Store(0)
	cShuffleMoves.Store(0)
	cMapLookups.Store(0)
}

func countDecide()    { cDecisions.Add(1) }
func countHandTotal() { cHandTotals.Add(1) }

// countCardValue compte une évaluation de carte, qui est aussi une consultation
// de map dans la version de référence.
func countCardValue() {
	cCardValues.Add(1)
	cMapLookups.Add(1)
}

func countMapLookup()         { cMapLookups.Add(1) }
func countShuffleMoves(n int) { cShuffleMoves.Add(uint64(n)) }
