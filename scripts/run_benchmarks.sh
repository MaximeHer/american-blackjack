#!/usr/bin/env bash
#
# Harnais de mesure du projet — critères 1 et 5 de la grille.
#
# Une seule commande produit l'intégralité du dossier de mesure : relevé du banc
# d'essai, vérification de l'oracle de non-régression, benchmarks Go comparés par
# benchstat, mesure de bout en bout par hyperfine, et profils CPU et allocations.
#
#   bash scripts/run_benchmarks.sh
#
# Tout est écrit dans results/<horodatage>/ et un résumé est affiché.
#
# Sous-commandes, pour n'exécuter qu'une étape :
#   bash scripts/run_benchmarks.sh hardware | test | bench | hyperfine | profile
#
set -uo pipefail

# ---------------------------------------------------------------- configuration

ROUNDS="${ROUNDS:-500000}"        # coups par exécution de bout en bout
WARMUP_ROUNDS="${WARMUP_ROUNDS:-50000}"
HF_RUNS="${HF_RUNS:-15}"          # exécutions hyperfine
HF_WARMUP="${HF_WARMUP:-3}"       # exécutions de chauffe hyperfine
BENCH_TIME="${BENCH_TIME:-2s}"    # protocole canonique
BENCH_COUNT="${BENCH_COUNT:-8}"
PROFILE_ITERS="${PROFILE_ITERS:-400x}"

PKG="./internal/blackjack"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# benchstat et hyperfine ne sont pas toujours dans le PATH hérité.
# $USER et $LOCALAPPDATA ne sont pas toujours definis selon le shell appelant :
# on les resout prudemment plutot que de supposer leur presence.
export PATH="$PATH:${HOME:-}/go/bin"
[ -n "${LOCALAPPDATA:-}" ] && export PATH="$PATH:$LOCALAPPDATA/Microsoft/WinGet/Links"
[ -d "${HOME:-}/AppData/Local/Microsoft/WinGet/Links" ] && export PATH="$PATH:$HOME/AppData/Local/Microsoft/WinGet/Links"

BIN_EXT=""
case "$(go env GOOS)" in windows) BIN_EXT=".exe" ;; esac

# ---------------------------------------------------------------------- sortie

STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="results/$STAMP"

c_ok=$'\033[32m'; c_warn=$'\033[33m'; c_err=$'\033[31m'; c_dim=$'\033[90m'; c_off=$'\033[0m'
if [ ! -t 1 ]; then c_ok=; c_warn=; c_err=; c_dim=; c_off=; fi

step()  { printf '\n%s==> %s%s\n' "$c_ok" "$*" "$c_off"; }
warn()  { printf '%s!! %s%s\n' "$c_warn" "$*" "$c_off"; }
fail()  { printf '%sXX %s%s\n' "$c_err" "$*" "$c_off"; }
note()  { printf '%s   %s%s\n' "$c_dim" "$*" "$c_off"; }

have() { command -v "$1" >/dev/null 2>&1; }

# ---------------------------------------------------------------------- étapes

do_hardware() {
  step "Relevé du banc d'essai"
  local f="$OUT/00-banc-essai.txt"
  if have powershell; then
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts/hardware.ps1 >"$f" 2>&1
  elif have pwsh; then
    pwsh -NoProfile -File scripts/hardware.ps1 >"$f" 2>&1
  else
    {
      echo "RELEVE DU BANC D'ESSAI (hors Windows, relevé réduit)"
      echo "Date : $(date '+%Y-%m-%d %H:%M:%S')"
      echo "Go   : $(go version)"
      echo "GOMAXPROCS : $(go env GOMAXPROCS)"
      command -v lscpu >/dev/null && lscpu
    } >"$f" 2>&1
  fi
  sed -n '1,14p' "$f" | sed 's/^/   /'
  note "complet dans $f"

  # Refus de mesurer sur batterie : le résultat serait faux, pas imprécis.
  if grep -q 'BATTERIE' "$f"; then
    fail "Machine sur batterie : la fréquence du processeur n'est pas stable."
    fail "Mesure interrompue. Branche le secteur et relance."
    exit 1
  fi
  if grep -q 'ATTENTION, charge' "$f"; then
    warn "La machine n'est pas au repos. Les mesures seront bruitées."
    warn "Voir docs/02-protocole-de-mesure.md section 4.1."
  fi
}

do_test() {
  step "Oracle de non-régression"
  local f="$OUT/10-oracle.txt"
  if go test "$PKG" -v -timeout 1800s >"$f" 2>&1; then
    grep -E 'avantage de la maison|écart au publié|écart-type par coup|dépassement du croupier|avantage via|écart  ' "$f" | sed 's/^ *//' | sed 's/^/   /'
    printf '%s   oracle VALIDE%s\n' "$c_ok" "$c_off"
  else
    fail "L'ORACLE A ÉCHOUÉ — la logique de jeu est cassée."
    fail "Aucune mesure de performance n'a de valeur dans cet état."
    grep -E '^\s+---\s+FAIL|FAIL:' "$f" | head -10 | sed 's/^/   /'
    note "détail dans $f"
    exit 1
  fi
}

do_bench() {
  step "Benchmarks Go (-benchtime $BENCH_TIME -count $BENCH_COUNT)"
  local f="$OUT/20-bench.txt"
  note "quelques minutes, ne touche pas à la machine"
  go test -run '^$' -bench . -benchmem \
    -benchtime "$BENCH_TIME" -count "$BENCH_COUNT" "$PKG" >"$f" 2>&1

  if ! grep -q '^Benchmark' "$f"; then
    fail "aucun benchmark n'a tourné"; sed -n '1,20p' "$f" | sed 's/^/   /'; return 1
  fi

  if have benchstat; then
    benchstat "$f" >"$OUT/21-bench-dispersion.txt" 2>&1
    sed -n '5,20p' "$OUT/21-bench-dispersion.txt" | sed 's/^/   /'

    if [ -f bench/baseline.txt ]; then
      benchstat bench/baseline.txt "$f" >"$OUT/22-vs-baseline.txt" 2>&1
      step "Comparaison au tag v0-baseline"
      sed -n '5,20p' "$OUT/22-vs-baseline.txt" | sed 's/^/   /'
      note "allocations et octets dans $OUT/22-vs-baseline.txt"
    else
      warn "bench/baseline.txt absent : pas de comparaison possible"
    fi
  else
    warn "benchstat absent — go install golang.org/x/perf/cmd/benchstat@latest"
    grep '^Benchmark' "$f" | sed 's/^/   /'
  fi
}

do_hyperfine() {
  step "Mesure de bout en bout (hyperfine)"
  local bin="$OUT/simulate$BIN_EXT"
  go build -o "$bin" ./cmd/simulate || { fail "compilation impossible"; return 1; }

  if ! have hyperfine; then
    warn "hyperfine absent — winget install sharkdp.hyperfine"
    warn "repli sur $HF_RUNS exécutions manuelles"
    local f="$OUT/30-debit-manuel.txt"
    for _ in $(seq 1 "$HF_RUNS"); do
      "$bin" -rounds "$ROUNDS" -warmup "$WARMUP_ROUNDS" -quiet
    done >"$f"
    note "$(sort -n "$f" | awk '{a[NR]=$1} END {printf "médiane %d coups/s sur %d exécutions", a[int((NR+1)/2)], NR}')"
    return 0
  fi

  note "--warmup $HF_WARMUP --runs $HF_RUNS, $ROUNDS coups par exécution"
  # --shell=none pour deux raisons. D'abord la correction : hyperfine passe
  # sinon par cmd.exe, qui refuse un chemin relatif à barres obliques. Ensuite
  # la méthode : exécuter le binaire directement retire le lancement du shell
  # du temps mesuré, ce qui est de toute façon ce qu'on cherche à mesurer.
  hyperfine \
    --shell=none \
    --warmup "$HF_WARMUP" \
    --runs "$HF_RUNS" \
    --style basic \
    --export-markdown "$OUT/30-hyperfine.md" \
    --export-json "$OUT/31-hyperfine.json" \
    --command-name "simulate ${ROUNDS} coups" \
    "$bin -rounds $ROUNDS -quiet" \
    2>&1 | tee "$OUT/32-hyperfine.txt" | sed 's/^/   /'
  # Pas de -warmup ici, volontairement. hyperfine chauffe déjà au niveau du
  # processus (--warmup), et cumuler les deux faussait l'interprétation : les
  # coups de chauffe du moteur entraient dans le temps mesuré, donc le temps
  # rapporté ne se divisait pas proprement par le nombre de coups annoncé.

  # Rapport détaillé du moteur lui-même : débit, mémoire, GC, ordonnanceur.
  "$bin" -rounds "$ROUNDS" -warmup "$WARMUP_ROUNDS"        >"$OUT/33-metriques.txt" 2>&1
  "$bin" -rounds "$ROUNDS" -warmup "$WARMUP_ROUNDS" -json  >"$OUT/34-metriques.json" 2>&1

  # Compteurs d'opérations : binaire séparé, jamais celui qui mesure le débit.
  local ibin="$OUT/simulate-instrumente$BIN_EXT"
  if go build -tags instrument -o "$ibin" ./cmd/simulate 2>/dev/null; then
    "$ibin" -rounds 200000 >"$OUT/35-operations.txt" 2>&1
    step "Opérations élémentaires (binaire instrumenté)"
    sed -n '/Opérations élémentaires/,$p' "$OUT/35-operations.txt" | sed -n '2,8p' | sed 's/^/   /'
  fi
}

do_profile() {
  step "Profils CPU et allocations"
  mkdir -p profiles
  go test -run '^$' -bench Simulate -benchtime "$PROFILE_ITERS" \
    -cpuprofile profiles/cpu.out -memprofile profiles/mem.out "$PKG" \
    >"$OUT/40-profile-run.txt" 2>&1

  if [ ! -s profiles/cpu.out ]; then
    warn "aucun profil produit"; return 1
  fi

  go tool pprof -top -cum -nodecount=20 profiles/cpu.out >"$OUT/41-cpu-top.txt" 2>&1
  go tool pprof -sample_index=alloc_objects -top -nodecount=12 profiles/mem.out >"$OUT/42-alloc-objets.txt" 2>&1
  go tool pprof -sample_index=alloc_space  -top -nodecount=12 profiles/mem.out >"$OUT/43-alloc-octets.txt" 2>&1

  # Annotations ligne par ligne des fonctions chaudes.
  : >"$OUT/44-lignes-chaudes.txt"
  for fn in 'Shoe..Shuffle' 'Hand..Total' 'Card..Value' 'decideBasic' 'PlayRound'; do
    {
      echo "======================================================================"
      echo "  $fn"
      echo "======================================================================"
      go tool pprof -list "$fn" profiles/cpu.out 2>/dev/null
      echo
    } >>"$OUT/44-lignes-chaudes.txt"
  done

  sed -n '6,16p' "$OUT/41-cpu-top.txt" | sed 's/^/   /'
  note "annotations ligne par ligne dans $OUT/44-lignes-chaudes.txt"
  note "flamegraph : go tool pprof -http=:9000 profiles/cpu.out"
}

do_summary() {
  step "Résumé"
  local f="$OUT/99-resume.md"
  {
    echo "# Campagne de mesure $STAMP"
    echo
    echo "Produite par \`bash scripts/run_benchmarks.sh\`."
    echo
    echo "- Révision : \`$(git rev-parse --short HEAD 2>/dev/null || echo inconnue)\`"
    echo "- Tag le plus proche : \`$(git describe --tags --abbrev=0 2>/dev/null || echo aucun)\`"
    echo "- Arbre de travail : $(if [ -z "$(git status --porcelain 2>/dev/null)" ]; then echo propre; else echo "MODIFIÉ — mesure non reproductible en l'état"; fi)"
    echo "- Protocole : \`-benchtime $BENCH_TIME -count $BENCH_COUNT\`, hyperfine \`--warmup $HF_WARMUP --runs $HF_RUNS\`"
    echo
    echo "## Bout en bout"
    echo
    [ -f "$OUT/30-hyperfine.md" ] && cat "$OUT/30-hyperfine.md"
    echo
    echo "## Benchmarks comparés au tag v0-baseline"
    echo
    echo '```'
    [ -f "$OUT/22-vs-baseline.txt" ] && sed -n '5,40p' "$OUT/22-vs-baseline.txt"
    echo '```'
    echo
    echo "## Fichiers"
    echo
    for x in "$OUT"/*; do echo "- \`$(basename "$x")\`"; done
  } >"$f"

  if [ -f "$OUT/30-hyperfine.md" ]; then
    grep -E '^\|' "$OUT/30-hyperfine.md" | sed 's/^/   /'
  fi
  printf '\n%s   Dossier de mesure : %s%s\n' "$c_ok" "$OUT" "$c_off"
  printf '%s   Résumé            : %s%s\n' "$c_ok" "$f" "$c_off"
}

# ------------------------------------------------------------------------ main

mkdir -p "$OUT"

case "${1:-all}" in
  hardware)  do_hardware ;;
  test)      do_test ;;
  bench)     do_bench ;;
  hyperfine) do_hyperfine ;;
  profile)   do_profile ;;
  all)
    printf '%sHarnais de mesure — american-blackjack%s\n' "$c_ok" "$c_off"
    note "sortie : $OUT"
    do_hardware
    do_test          # l'oracle d'abord : mesurer un code cassé n'a aucun sens
    do_bench
    do_hyperfine
    do_profile
    do_summary
    ;;
  *)
    echo "usage : bash scripts/run_benchmarks.sh [all|hardware|test|bench|hyperfine|profile]"
    exit 2
    ;;
esac
