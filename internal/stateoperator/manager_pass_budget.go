package stateoperator

import (
	"slices"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
)

// groupWriteBudget bounds one manager pass to a number of groups with writes, so a
// pass over tens of thousands of groups cannot hold back a finalizer removal or a
// status update for minutes. Groups left out are planned again by the next pass,
// which the status writes of this pass trigger right away.
//
// Groups being deleted always come first. The other groups are taken in name order
// starting after the last group the previous pass took, so groups whose writes keep
// failing cannot use up the whole budget every pass.
type groupWriteBudget struct {
	maxGroups int
	// after is the last non-deleting group name taken by the previous pass.
	after string
}

// limit removes the writes of every group past the budget from plan and returns how
// many groups were deferred. The cloud status write is never removed.
func (budget *groupWriteBudget) limit(plan *ManagerPlan, snapshot *ManagerSnapshot) int {
	if budget.maxGroups <= 0 {
		return 0
	}
	names := groupsWithWrites(plan)
	if len(names) <= budget.maxGroups {
		budget.after = ""
		return 0
	}
	deleting := map[string]bool{}
	for groupIndex := range snapshot.LocalGroups {
		group := &snapshot.LocalGroups[groupIndex]
		if group.DeletionTimestamp != nil {
			deleting[group.Name] = true
		}
	}
	var first, rest []string
	for name := range names {
		if deleting[name] {
			first = append(first, name)
			continue
		}
		rest = append(rest, name)
	}
	slices.Sort(first)
	slices.Sort(rest)
	start, _ := slices.BinarySearch(rest, budget.after)
	if start < len(rest) && rest[start] == budget.after {
		start++
	}
	order := slices.Concat(first, rest[start:], rest[:start])

	keep := make(map[string]bool, budget.maxGroups)
	for _, name := range order[:budget.maxGroups] {
		keep[name] = true
		if !deleting[name] {
			budget.after = name
		}
	}
	dropGroupWrites(plan, keep)
	return len(names) - budget.maxGroups
}

// groupsWithWrites returns the names of all groups that have a Kubernetes or NSX write in plan.
func groupsWithWrites(plan *ManagerPlan) map[string]struct{} {
	names := map[string]struct{}{}
	addKeys := func(keys []kubeapi.BatchKey) {
		for _, key := range keys {
			names[key.Name] = struct{}{}
		}
	}
	writes := &plan.KubeWrites
	addKeys(mapKeys(writes.GroupCreates))
	addKeys(mapKeys(writes.GroupUpdates))
	addKeys(mapKeys(writes.GroupDeletes))
	addKeys(mapKeys(writes.GroupStatusUpdates))
	addKeys(mapKeys(writes.GroupFinalizerPatches))
	addKeys(mapKeys(writes.GroupStatusesAfterGroupWrite))
	addKeys(mapKeys(writes.GroupFinalizersAfterGroupWrite))
	addKeys(mapKeys(writes.GroupFinalizersAfterStatusWrite))
	for _, key := range nsxWriteKeys(&plan.NSXWrites) {
		names[plan.writeFailureTargets[key].Name] = struct{}{}
	}
	return names
}

func dropGroupWrites(plan *ManagerPlan, keep map[string]bool) {
	writes := &plan.KubeWrites
	dropByName := func(key kubeapi.BatchKey) bool { return !keep[key.Name] }
	deleteKeysFunc(writes.GroupCreates, dropByName)
	deleteKeysFunc(writes.GroupUpdates, dropByName)
	deleteKeysFunc(writes.GroupDeletes, dropByName)
	deleteKeysFunc(writes.GroupStatusUpdates, dropByName)
	deleteKeysFunc(writes.GroupFinalizerPatches, dropByName)
	deleteKeysFunc(writes.GroupStatusesAfterGroupWrite, dropByName)
	deleteKeysFunc(writes.GroupFinalizersAfterGroupWrite, dropByName)
	deleteKeysFunc(writes.GroupFinalizersAfterStatusWrite, dropByName)

	dropNSX := func(key BindingKey) bool { return !keep[plan.writeFailureTargets[key].Name] }
	deleteKeysFunc(plan.NSXWrites.Patches, dropNSX)
	deleteKeysFunc(plan.NSXWrites.Puts, dropNSX)
	deleteKeysFunc(plan.NSXWrites.Deletes, dropNSX)

	// The summary lists mirror the write maps; keep them in step so the legacy
	// fallback in applyManagerPlan can never bring a dropped write back.
	keptDeletes := map[string]bool{}
	for _, deletion := range plan.NSXWrites.Deletes {
		keptDeletes[deletion.GroupID] = true
	}
	dropName := func(name string) bool { return !keep[name] }
	plan.ObserveUpserts = slices.DeleteFunc(plan.ObserveUpserts, func(group nsxv1alpha.NSXGroup) bool { return !keep[group.Name] })
	plan.ManagedWrites = slices.DeleteFunc(plan.ManagedWrites, func(write ManagedGroupWrite) bool { return !keep[write.Name] })
	plan.ManagedDeletes = slices.DeleteFunc(plan.ManagedDeletes, func(deletion ManagedGroupDelete) bool { return !keptDeletes[deletion.GroupID] })
	plan.ObserveFinalizerRemovals = slices.DeleteFunc(plan.ObserveFinalizerRemovals, dropName)
	plan.ManagedFinalizerRemovals = slices.DeleteFunc(plan.ManagedFinalizerRemovals, dropName)
	plan.ObserveDeletes = slices.DeleteFunc(plan.ObserveDeletes, dropName)
	plan.GroupStatuses = slices.DeleteFunc(plan.GroupStatuses, func(status GroupStatusPlan) bool { return !keep[status.Name] })
}

func nsxWriteKeys(writes *ManagerNSXWritePlan) []BindingKey {
	keys := mapKeys(writes.Patches)
	keys = append(keys, mapKeys(writes.Puts)...)
	return append(keys, mapKeys(writes.Deletes)...)
}

func mapKeys[K comparable, V any](items map[K]V) []K {
	keys := make([]K, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

func deleteKeysFunc[K comparable, V any](items map[K]V, drop func(K) bool) {
	for key := range items {
		if drop(key) {
			delete(items, key)
		}
	}
}
