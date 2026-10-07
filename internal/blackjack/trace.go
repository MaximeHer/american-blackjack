package blackjack

import "fmt"

// Trace collecte le récit d'un coup, carte par carte et décision par décision.
//
// Le journal est une vraie fonctionnalité : l'interface web l'affiche, via
// /api/sample-round. Le défaut de la version de référence n'était donc pas de
// le produire, mais de le produire SANS QUE L'APPELANT L'AIT DEMANDÉ — et donc
// aussi dans la boucle de simulation, qui ne le lit jamais.
//
// C'est la forme la plus répandue de gaspillage en production : un code partagé
// entre deux usages paie le coût du plus exigeant des deux.
//
// # Pourquoi la garde est au site d'appel
//
// Un *Trace nil désactive la narration, et le test de nullité est écrit AU SITE
// D'APPEL plutôt que dans add(). Ce n'est pas un choix de style.
//
// Go autorise l'appel d'une méthode sur un récepteur nil, et l'on pourrait donc
// tester t == nil à l'intérieur de add(). Mais l'appel aurait quand même lieu,
// donc ses arguments seraient quand même évalués : les Label() construiraient
// leurs chaînes, et les arguments variadiques seraient empaquetés dans un []any
// alloué sur le tas. L'essentiel du coût serait payé pour rien.
//
// Écrire `if tr != nil { tr.add(…) }` supprime l'évaluation elle-même, et la
// branche est parfaitement prédite par le processeur puisque sa valeur ne change
// jamais au cours d'une exécution.
type Trace struct {
	Lines []string
}

// NewTrace ouvre un collecteur de récit.
func NewTrace() *Trace { return &Trace{} }

// add ajoute une ligne au récit.
//
// À n'appeler que sous la garde `if tr != nil`, pour la raison exposée plus
// haut : sans elle, les arguments seraient évalués et empaquetés même quand la
// narration est désactivée.
func (t *Trace) add(format string, args ...any) {
	t.Lines = append(t.Lines, fmt.Sprintf(format, args...))
}
