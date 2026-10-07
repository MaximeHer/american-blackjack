// Package metrics instrumente une charge de travail SANS la modifier.
//
// Principe directeur : toutes les grandeurs sont lues dans les compteurs du
// runtime Go avant et après la charge, jamais pendant. Le coût de la mesure est
// donc constant — deux appels à metrics.Read — et strictement indépendant du
// nombre d'unités traitées. La boucle mesurée reste exactement telle qu'elle
// s'exécute en production.
//
// C'est la condition de validité du protocole : une boucle qui incrémente des
// compteurs ne mesure plus le moteur, elle mesure le moteur plus son
// instrumentation.
//
// # Précision des compteurs d'allocation
//
// /gc/heap/allocs:objects et :bytes sont alimentés par des caches propres à
// chaque processeur logique, que le runtime vide paresseusement. Les toutes
// dernières allocations peuvent donc ne pas être comptées au moment de la
// lecture — mesuré à 5 objets sur 2 000, soit 0,25 %.
//
// Le retard est un petit nombre ABSOLU d'objets, borné par le nombre de
// processeurs logiques, et non une fraction du total. Il est donc négligeable
// sur une charge réelle (5 objets sur 23 millions) mais sensible sur quelques
// milliers d'unités. Pour un comptage exact à l'objet près sur une petite
// charge, utiliser `go test -benchmem`, qui force un vidage.
package metrics

import (
	"math"
	"runtime"
	"runtime/metrics"
	"time"
)

// Noms des compteurs lus. Tous sont disponibles depuis Go 1.20.
const (
	mCPUTotal   = "/cpu/classes/total:cpu-seconds"
	mCPUUser    = "/cpu/classes/user:cpu-seconds"
	mCPUGC      = "/cpu/classes/gc/total:cpu-seconds"
	mCPUIdle    = "/cpu/classes/idle:cpu-seconds"
	mAllocBytes = "/gc/heap/allocs:bytes"
	mAllocObjs  = "/gc/heap/allocs:objects"
	mGCCycles   = "/gc/cycles/total:gc-cycles"
	mGCPauses   = "/gc/pauses:seconds"
	mSchedLat   = "/sched/latencies:seconds"
	mGoCreated  = "/sched/goroutines-created:goroutines"
	mHeapObjs   = "/gc/heap/objects:objects"
)

var sampleNames = []string{
	mCPUTotal, mCPUUser, mCPUGC, mCPUIdle,
	mAllocBytes, mAllocObjs, mGCCycles, mGCPauses,
	mSchedLat, mGoCreated, mHeapObjs,
}

// snapshot est une lecture instantanée des compteurs du runtime.
type snapshot struct {
	wall time.Time

	cpuTotal, cpuUser, cpuGC, cpuIdle float64
	allocBytes, allocObjs             uint64
	gcCycles, goCreated, heapObjs     uint64
	gcPauses, schedLat                *metrics.Float64Histogram
}

// Run encadre une charge de travail mesurée.
type Run struct {
	before snapshot
}

// Begin relève l'état des compteurs et démarre le chronomètre.
func Begin() *Run {
	return &Run{before: read()}
}

// End relève l'état final et construit le rapport. units est le nombre
// d'unités de travail traitées — ici des coups de blackjack — et sert de
// dénominateur aux grandeurs rapportées par unité.
func (r *Run) End(units int64) Report {
	after := read()
	return build(r.before, after, units)
}

// read lit tous les compteurs en une passe.
//
// Les histogrammes renvoyés par metrics.Read pointent vers des tampons que le
// runtime peut réutiliser d'un appel à l'autre : ils sont donc recopiés ici,
// sans quoi la lecture initiale serait écrasée par la finale.
func read() snapshot {
	samples := make([]metrics.Sample, len(sampleNames))
	for i, n := range sampleNames {
		samples[i].Name = n
	}
	metrics.Read(samples)

	s := snapshot{wall: time.Now()}
	for _, sm := range samples {
		switch sm.Name {
		case mCPUTotal:
			s.cpuTotal = sm.Value.Float64()
		case mCPUUser:
			s.cpuUser = sm.Value.Float64()
		case mCPUGC:
			s.cpuGC = sm.Value.Float64()
		case mCPUIdle:
			s.cpuIdle = sm.Value.Float64()
		case mAllocBytes:
			s.allocBytes = sm.Value.Uint64()
		case mAllocObjs:
			s.allocObjs = sm.Value.Uint64()
		case mGCCycles:
			s.gcCycles = sm.Value.Uint64()
		case mGoCreated:
			s.goCreated = sm.Value.Uint64()
		case mHeapObjs:
			s.heapObjs = sm.Value.Uint64()
		case mGCPauses:
			s.gcPauses = copyHist(sm.Value.Float64Histogram())
		case mSchedLat:
			s.schedLat = copyHist(sm.Value.Float64Histogram())
		}
	}
	return s
}

// Report rassemble les grandeurs mesurées sur l'intervalle.
type Report struct {
	Units int64 `json:"units"`

	// --- Temps ---
	Wall     time.Duration `json:"-"`
	WallSec  float64       `json:"wallSeconds"`
	CPUBusy  time.Duration `json:"-"`
	CPUUser  time.Duration `json:"-"`
	CPUGC    time.Duration `json:"-"`
	NsPerOp  float64       `json:"nsPerOp"`
	OpsPerS  float64       `json:"opsPerSecond"`
	CPUNsOp  float64       `json:"cpuNsPerOp"`
	Parallel float64       `json:"parallelism"`

	// --- Mémoire ---
	BytesAlloc     uint64  `json:"bytesAllocated"`
	ObjectsAlloc   uint64  `json:"objectsAllocated"`
	BytesPerOp     float64 `json:"bytesPerOp"`
	AllocsPerOp    float64 `json:"allocsPerOp"`
	AllocRateMBs   float64 `json:"allocRateMBPerSecond"`
	HeapObjectsIn  uint64  `json:"heapObjectsStart"`
	HeapObjectsEnd uint64  `json:"heapObjectsEnd"`

	// --- Ramasse-miettes ---
	GCCycles       uint64        `json:"gcCycles"`
	GCPerOp        float64       `json:"gcCyclesPerMillionOps"`
	GCPauseTotal   time.Duration `json:"-"`
	GCPauseP50     time.Duration `json:"-"`
	GCPauseP99     time.Duration `json:"-"`
	GCPauseMax     time.Duration `json:"-"`
	GCPauseTotalMs float64       `json:"gcPauseTotalMs"`
	GCPauseP99Us   float64       `json:"gcPauseP99Us"`
	GCWallShare    float64       `json:"gcWallSharePercent"`
	GCCPUShare     float64       `json:"gcCpuSharePercent"`

	// --- Ordonnanceur ---
	GoroutinesCreated uint64        `json:"goroutinesCreated"`
	SchedLatP50       time.Duration `json:"-"`
	SchedLatP99       time.Duration `json:"-"`
	SchedLatP99Us     float64       `json:"schedLatencyP99Us"`

	// --- Environnement ---
	GOMAXPROCS int    `json:"gomaxprocs"`
	NumCPU     int    `json:"numCPU"`
	GoVersion  string `json:"goVersion"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
}

func build(a, b snapshot, units int64) Report {
	wall := b.wall.Sub(a.wall)
	n := float64(units)
	if n <= 0 {
		n = 1
	}

	cpuUser := secs(b.cpuUser - a.cpuUser)
	cpuGC := secs(b.cpuGC - a.cpuGC)
	// Le temps « occupé » exclut l'inactivité : c'est le travail réellement
	// fourni par les coeurs, toutes classes confondues.
	cpuBusy := cpuUser + cpuGC + secs((b.cpuTotal-a.cpuTotal)-(b.cpuUser-a.cpuUser)-(b.cpuGC-a.cpuGC)-(b.cpuIdle-a.cpuIdle))

	pauses := diffHist(a.gcPauses, b.gcPauses)
	sched := diffHist(a.schedLat, b.schedLat)

	bytes := b.allocBytes - a.allocBytes
	objs := b.allocObjs - a.allocObjs
	pauseTotal := histSum(pauses)

	r := Report{
		Units:   units,
		Wall:    wall,
		WallSec: wall.Seconds(),
		CPUBusy: cpuBusy,
		CPUUser: cpuUser,
		CPUGC:   cpuGC,
		NsPerOp: float64(wall.Nanoseconds()) / n,
		CPUNsOp: float64(cpuBusy.Nanoseconds()) / n,

		BytesAlloc:     bytes,
		ObjectsAlloc:   objs,
		BytesPerOp:     float64(bytes) / n,
		AllocsPerOp:    float64(objs) / n,
		HeapObjectsIn:  a.heapObjs,
		HeapObjectsEnd: b.heapObjs,

		GCCycles:       b.gcCycles - a.gcCycles,
		GCPauseTotal:   pauseTotal,
		GCPauseP50:     quantile(pauses, 0.50),
		GCPauseP99:     quantile(pauses, 0.99),
		GCPauseMax:     histMax(pauses),
		GCPauseTotalMs: float64(pauseTotal.Microseconds()) / 1000,

		GoroutinesCreated: b.goCreated - a.goCreated,
		SchedLatP50:       quantile(sched, 0.50),
		SchedLatP99:       quantile(sched, 0.99),

		GOMAXPROCS: runtime.GOMAXPROCS(0),
		NumCPU:     runtime.NumCPU(),
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}

	if wall > 0 {
		r.OpsPerS = n / wall.Seconds()
		// Parallélisme effectif : combien de coeurs ont travaillé en moyenne.
		// 1,0 signifie un seul coeur occupé en continu.
		r.Parallel = cpuBusy.Seconds() / wall.Seconds()
		r.AllocRateMBs = float64(bytes) / (1 << 20) / wall.Seconds()
		r.GCWallShare = pauseTotal.Seconds() / wall.Seconds() * 100
	}
	if cpuBusy > 0 {
		r.GCCPUShare = cpuGC.Seconds() / cpuBusy.Seconds() * 100
	}
	if units > 0 {
		r.GCPerOp = float64(r.GCCycles) * 1e6 / n
	}
	r.GCPauseP99Us = float64(r.GCPauseP99.Nanoseconds()) / 1000
	r.SchedLatP99Us = float64(r.SchedLatP99.Nanoseconds()) / 1000

	return r
}

func secs(f float64) time.Duration {
	if f < 0 || math.IsNaN(f) {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

func copyHist(h *metrics.Float64Histogram) *metrics.Float64Histogram {
	if h == nil {
		return nil
	}
	out := &metrics.Float64Histogram{
		Counts:  make([]uint64, len(h.Counts)),
		Buckets: make([]float64, len(h.Buckets)),
	}
	copy(out.Counts, h.Counts)
	copy(out.Buckets, h.Buckets)
	return out
}

// diffHist soustrait deux histogrammes cumulatifs pour obtenir la distribution
// propre à l'intervalle mesuré.
func diffHist(a, b *metrics.Float64Histogram) *metrics.Float64Histogram {
	if b == nil {
		return nil
	}
	out := copyHist(b)
	if a == nil {
		return out
	}
	for i := range out.Counts {
		if i < len(a.Counts) && out.Counts[i] >= a.Counts[i] {
			out.Counts[i] -= a.Counts[i]
		}
	}
	return out
}

func histCount(h *metrics.Float64Histogram) uint64 {
	if h == nil {
		return 0
	}
	var t uint64
	for _, c := range h.Counts {
		t += c
	}
	return t
}

// histSum estime la somme des observations en attribuant à chaque seau le
// milieu de son intervalle. C'est une approximation, suffisante pour un total
// de pauses dont les seaux sont resserrés.
func histSum(h *metrics.Float64Histogram) time.Duration {
	if h == nil {
		return 0
	}
	var total float64
	for i, c := range h.Counts {
		if c == 0 {
			continue
		}
		total += mid(h, i) * float64(c)
	}
	return secs(total)
}

func histMax(h *metrics.Float64Histogram) time.Duration {
	if h == nil {
		return 0
	}
	for i := len(h.Counts) - 1; i >= 0; i-- {
		if h.Counts[i] > 0 {
			return secs(upper(h, i))
		}
	}
	return 0
}

// quantile renvoie la borne supérieure du seau contenant le quantile demandé.
// C'est une estimation conservatrice : la valeur réelle est inférieure ou
// égale.
func quantile(h *metrics.Float64Histogram, q float64) time.Duration {
	total := histCount(h)
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * q))
	var cum uint64
	for i, c := range h.Counts {
		cum += c
		if cum >= target {
			return secs(upper(h, i))
		}
	}
	return 0
}

// upper renvoie la borne supérieure finie du seau i.
func upper(h *metrics.Float64Histogram, i int) float64 {
	hi := h.Buckets[i+1]
	if math.IsInf(hi, 1) {
		return h.Buckets[i]
	}
	return hi
}

// mid renvoie le milieu du seau i, en traitant les bornes infinies.
func mid(h *metrics.Float64Histogram, i int) float64 {
	lo, hi := h.Buckets[i], h.Buckets[i+1]
	if math.IsInf(lo, -1) {
		lo = hi
	}
	if math.IsInf(hi, 1) {
		hi = lo
	}
	return (lo + hi) / 2
}

// newTestHist construit un histogramme à des fins de test.
func newTestHist(buckets []float64, counts []uint64) *metrics.Float64Histogram {
	return &metrics.Float64Histogram{Buckets: buckets, Counts: counts}
}

// GCCyclesSoFar renvoie le nombre de cycles de ramasse-miettes écoulés depuis
// le début de la mesure, sans la clore.
//
// Utile pour un affichage de progression : la mesure globale reste ouverte et
// son relevé final n'est pas perturbé.
func (r *Run) GCCyclesSoFar() uint64 {
	s := []metrics.Sample{{Name: mGCCycles}}
	metrics.Read(s)
	now := s[0].Value.Uint64()
	if now < r.before.gcCycles {
		return 0
	}
	return now - r.before.gcCycles
}
