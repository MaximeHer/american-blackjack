# Harnais de mesure — critères 1 et 5 de la grille.
#
# La commande de référence est UNE SEULE :
#
#     make measure
#
# qui produit l'intégralité du dossier de mesure dans results/<horodatage>/.
#
# `make` n'est pas installé sur la machine de développement (Windows). Toutes
# les cibles ci-dessous délèguent donc à scripts/run_benchmarks.sh, qui peut
# s'invoquer directement et reste le point d'entrée canonique :
#
#     bash scripts/run_benchmarks.sh
#
# Le Makefile existe pour les environnements qui ont make, et pour servir de
# table des matières des commandes du projet.

SHELL := /usr/bin/env bash
HARNESS := bash scripts/run_benchmarks.sh
PKG := ./internal/blackjack

.DEFAULT_GOAL := help
.PHONY: help measure hardware test test-instrument bench bench-save hyperfine \
        profile profile-top profile-lines flame serve play oracle vet fmt \
        clean-results check

## help : liste les cibles
help:
	@echo "Harnais de mesure — american-blackjack"
	@echo
	@echo "  make measure         campagne complète, une seule commande (critère 5)"
	@echo
	@echo "  make check           vet + gofmt + tests, dans les deux builds"
	@echo "  make oracle          l'oracle de non-régression seul"
	@echo "  make hardware        relevé du banc d'essai (critère 1)"
	@echo "  make bench           benchmarks au protocole canonique"
	@echo "  make bench-save F=x  enregistre les benchmarks dans bench/x.txt"
	@echo "  make hyperfine       mesure de bout en bout (critère 1)"
	@echo "  make profile         profils CPU et allocations (critère 2)"
	@echo "  make profile-top     où passe le temps, par fonction"
	@echo "  make profile-lines   annotations ligne par ligne"
	@echo "  make flame           flamegraph interactif dans le navigateur"
	@echo
	@echo "  make serve           lance la table de blackjack sur :8090"
	@echo "  make play            simulation en ligne de commande"
	@echo
	@echo "Variables : ROUNDS, BENCH_TIME, BENCH_COUNT, HF_RUNS, HF_WARMUP"

## measure : LA commande du critère 5 — tout le dossier de mesure en une fois
measure:
	@$(HARNESS) all

hardware:
	@$(HARNESS) hardware

bench:
	@$(HARNESS) bench

hyperfine:
	@$(HARNESS) hyperfine

profile:
	@$(HARNESS) profile

## test : la suite complète, build par défaut
test:
	go test ./... -timeout 1800s

## test-instrument : la même suite avec les compteurs d'opérations
test-instrument:
	go test -tags instrument ./... -timeout 1800s

## oracle : le seul test qui conditionne la validité de toute mesure
oracle:
	go test $(PKG) -run 'TestHouseEdgeOracle|TestTableCrossValidation|TestDeterminisme' -v -timeout 900s

## vet : analyse statique, dans les deux builds
vet:
	go vet ./...
	go vet -tags instrument ./...

## fmt : met en forme le code
fmt:
	gofmt -w .

## check : tout ce qui doit passer avant un commit
check: fmt vet test test-instrument
	@echo "OK — prêt à committer"

## bench-save : enregistre un relevé nommé, pour comparaison ultérieure
## usage : make bench-save F=palier-04
bench-save:
	@test -n "$(F)" || { echo "usage : make bench-save F=<nom>"; exit 2; }
	go test -run '^$$' -bench . -benchmem -benchtime 2s -count 8 $(PKG) > bench/$(F).txt
	@benchstat bench/baseline.txt bench/$(F).txt

profile-top: profiles/cpu.out
	@echo "--- CPU, cumulé par fonction ---"
	@go tool pprof -top -cum -nodecount=18 profiles/cpu.out
	@echo
	@echo "--- allocations, par objets ---"
	@go tool pprof -sample_index=alloc_objects -top -nodecount=10 profiles/mem.out
	@echo
	@echo "--- allocations, par octets ---"
	@go tool pprof -sample_index=alloc_space -top -nodecount=10 profiles/mem.out

profile-lines: profiles/cpu.out
	@for fn in 'Shoe..Shuffle' 'Hand..Total' 'Card..Value' 'decideBasic'; do \
	  echo "=== $$fn ==="; \
	  go tool pprof -list "$$fn" profiles/cpu.out; \
	  echo; \
	done

## flame : flamegraph interactif, sans dépendance Graphviz
flame: profiles/cpu.out
	@echo "Onglet Flame Graph sur http://localhost:9000 — Ctrl+C pour arrêter"
	go tool pprof -http=:9000 profiles/cpu.out

profiles/cpu.out:
	@$(HARNESS) profile

serve:
	go run ./cmd/server

play:
	go run ./cmd/simulate -rounds 500000 -warmup 50000

clean-results:
	rm -rf results profiles
