package main

type UpdatePlan struct {
	ToAdd     []GitHubSource
	ToUpdate  []GitHubSource
	ToRemove  []Template
	Unchanged []Template
}

func PlanUpdate(sources []GitHubSource, current []Template) UpdatePlan {
	currentByID := make(map[TemplateSourceID]Template, len(current))
	for _, template := range current {
		currentByID[template.SourceID()] = template
	}

	sourceByID := make(map[TemplateSourceID]GitHubSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID()] = source
	}

	var plan UpdatePlan
	for _, source := range sources {
		template, exists := currentByID[source.ID()]
		if !exists {
			plan.ToAdd = append(plan.ToAdd, source)
			continue
		}

		if template.Ref != source.SHA {
			plan.ToUpdate = append(plan.ToUpdate, source)
			continue
		}

		plan.Unchanged = append(plan.Unchanged, template)
	}

	for _, template := range current {
		if _, exists := sourceByID[template.SourceID()]; !exists {
			plan.ToRemove = append(plan.ToRemove, template)
		}
	}

	return plan
}
