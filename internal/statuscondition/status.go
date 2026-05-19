package statuscondition

import (
	"fmt"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConditionUpdate struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

type StatusWriteDecision struct {
	Needed      bool
	Reason      string
	DriftFields []string
}

func RemotePresent(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionRemotePresent, Status: status, Reason: reason, Message: message}
}

func Reachable(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionReachable, Status: status, Reason: reason, Message: message}
}

func Swept(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionSwept, Status: status, Reason: reason, Message: message}
}

func SpecMatchesRemote(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionSpecMatchesRemote, Status: status, Reason: reason, Message: message}
}

func UnsupportedExpression(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionUnsupportedExpression, Status: status, Reason: reason, Message: message}
}

func Realized(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionRealized, Status: status, Reason: reason, Message: message}
}

func Applying(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionApplying, Status: status, Reason: reason, Message: message}
}

func Deleting(status metav1.ConditionStatus, reason string, message string) ConditionUpdate {
	return ConditionUpdate{Type: nsxv1alpha.ConditionDeleting, Status: status, Reason: reason, Message: message}
}

func Synced(
	remotePresent metav1.ConditionStatus,
	specMatchesRemote metav1.ConditionStatus,
	unsupportedExpression metav1.ConditionStatus,
	realized metav1.ConditionStatus,
	reason string,
	message string,
) ConditionUpdate {
	status := metav1.ConditionTrue
	if remotePresent == metav1.ConditionFalse ||
		specMatchesRemote == metav1.ConditionFalse ||
		unsupportedExpression == metav1.ConditionTrue ||
		realized == metav1.ConditionFalse {
		status = metav1.ConditionFalse
	} else if remotePresent == metav1.ConditionUnknown ||
		specMatchesRemote == metav1.ConditionUnknown ||
		unsupportedExpression == metav1.ConditionUnknown ||
		realized == metav1.ConditionUnknown {
		status = metav1.ConditionUnknown
	}
	return ConditionUpdate{Type: nsxv1alpha.ConditionSynced, Status: status, Reason: reason, Message: message}
}

func BuildGroupStatus(
	previous nsxv1alpha.NSXGroupStatus,
	observedGeneration int64,
	now time.Time,
	updates ...ConditionUpdate,
) (nsxv1alpha.NSXGroupStatus, error) {
	conditions, err := buildConditions(previous.Conditions, groupConditionOrder, observedGeneration, now, updates)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, err
	}
	unsupportedReason, err := unsupportedReasonFromUpdates(updates)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, err
	}
	return nsxv1alpha.NSXGroupStatus{UnsupportedReason: unsupportedReason, Conditions: conditions}, nil
}

func BuildNetworkCloudStatus(
	previous nsxv1alpha.NSXNetworkCloudStatus,
	observedGeneration int64,
	now time.Time,
	updates ...ConditionUpdate,
) (nsxv1alpha.NSXNetworkCloudStatus, error) {
	conditions, err := buildConditions(previous.Conditions, cloudConditionOrder, observedGeneration, now, updates)
	if err != nil {
		return nsxv1alpha.NSXNetworkCloudStatus{}, err
	}
	return nsxv1alpha.NSXNetworkCloudStatus{Conditions: conditions}, nil
}

func CompareGroupStatus(current nsxv1alpha.NSXGroupStatus, desired nsxv1alpha.NSXGroupStatus) StatusWriteDecision {
	if current.UnsupportedReason != desired.UnsupportedReason {
		return StatusWriteDecision{Needed: true, Reason: "unsupported_reason_changed", DriftFields: []string{"unsupportedReason"}}
	}
	return compareConditions(current.Conditions, desired.Conditions)
}

func CompareNetworkCloudStatus(current nsxv1alpha.NSXNetworkCloudStatus, desired nsxv1alpha.NSXNetworkCloudStatus) StatusWriteDecision {
	return compareConditions(current.Conditions, desired.Conditions)
}

var groupConditionOrder = []string{
	nsxv1alpha.ConditionRemotePresent,
	nsxv1alpha.ConditionSpecMatchesRemote,
	nsxv1alpha.ConditionUnsupportedExpression,
	nsxv1alpha.ConditionRealized,
	nsxv1alpha.ConditionSynced,
	nsxv1alpha.ConditionApplying,
	nsxv1alpha.ConditionDeleting,
}

var cloudConditionOrder = []string{
	nsxv1alpha.ConditionReachable,
	nsxv1alpha.ConditionSwept,
}

func compareConditions(current []metav1.Condition, desired []metav1.Condition) StatusWriteDecision {
	if len(current) != len(desired) {
		return StatusWriteDecision{Needed: true, Reason: "condition_count_changed", DriftFields: []string{"conditions"}}
	}
	for index := range current {
		currentCondition := current[index]
		desiredCondition := desired[index]
		if currentCondition.Type != desiredCondition.Type {
			return StatusWriteDecision{Needed: true, Reason: "condition_type_changed", DriftFields: []string{"conditions.type"}}
		}
		if currentCondition.Status != desiredCondition.Status {
			return StatusWriteDecision{Needed: true, Reason: "condition_status_changed", DriftFields: []string{"conditions.status"}}
		}
		if currentCondition.Reason != desiredCondition.Reason {
			return StatusWriteDecision{Needed: true, Reason: "condition_reason_changed", DriftFields: []string{"conditions.reason"}}
		}
		if currentCondition.Message != desiredCondition.Message {
			return StatusWriteDecision{Needed: true, Reason: "condition_message_changed", DriftFields: []string{"conditions.message"}}
		}
		if currentCondition.ObservedGeneration != desiredCondition.ObservedGeneration {
			return StatusWriteDecision{Needed: true, Reason: "condition_observed_generation_changed", DriftFields: []string{"conditions.observedGeneration"}}
		}
		if !currentCondition.LastTransitionTime.Equal(&desiredCondition.LastTransitionTime) {
			return StatusWriteDecision{Needed: true, Reason: "condition_last_transition_time_changed", DriftFields: []string{"conditions.lastTransitionTime"}}
		}
	}
	return StatusWriteDecision{Needed: false, Reason: "status_equal"}
}

func buildConditions(
	previous []metav1.Condition,
	conditionOrder []string,
	observedGeneration int64,
	now time.Time,
	updates []ConditionUpdate,
) ([]metav1.Condition, error) {
	previousByType := make(map[string]metav1.Condition, len(previous))
	for _, condition := range previous {
		previousByType[condition.Type] = condition
	}
	orderIndexByType := make(map[string]int, len(conditionOrder))
	for index, conditionType := range conditionOrder {
		orderIndexByType[conditionType] = index
	}
	updateByType := make(map[string]ConditionUpdate, len(updates))
	for _, update := range updates {
		if _, ok := orderIndexByType[update.Type]; !ok {
			return nil, fmt.Errorf("unsupported condition type %q", update.Type)
		}
		if _, exists := updateByType[update.Type]; exists {
			return nil, fmt.Errorf("duplicate condition update type %q", update.Type)
		}
		updateByType[update.Type] = update
	}
	conditions := make([]metav1.Condition, 0, len(updates))
	for _, conditionType := range conditionOrder {
		update, ok := updateByType[conditionType]
		if !ok {
			continue
		}
		lastTransitionTime := metav1.NewTime(now)
		if previousCondition, exists := previousByType[update.Type]; exists && previousCondition.Status == update.Status {
			lastTransitionTime = previousCondition.LastTransitionTime
		}
		conditions = append(conditions, metav1.Condition{
			Type:               update.Type,
			Status:             update.Status,
			ObservedGeneration: observedGeneration,
			LastTransitionTime: lastTransitionTime,
			Reason:             update.Reason,
			Message:            update.Message,
		})
	}
	return conditions, nil
}

func unsupportedReasonFromUpdates(updates []ConditionUpdate) (nsxv1alpha.UnsupportedExpressionReason, error) {
	for _, update := range updates {
		if update.Type != nsxv1alpha.ConditionUnsupportedExpression {
			continue
		}
		if update.Status != metav1.ConditionTrue {
			return "", nil
		}
		reason := nsxv1alpha.UnsupportedExpressionReason(update.Reason)
		if !isStatusUnsupportedReason(reason) {
			return "", fmt.Errorf("unsupported expression condition reason %q is not a status unsupported reason enum value", update.Reason)
		}
		return reason, nil
	}
	return "", nil
}

func isStatusUnsupportedReason(reason nsxv1alpha.UnsupportedExpressionReason) bool {
	switch reason {
	case nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType,
		nsxv1alpha.UnsupportedExpressionReasonMultipleIPAddressExpressions,
		nsxv1alpha.UnsupportedExpressionReasonMultiplePathExpressions,
		nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression,
		nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression,
		nsxv1alpha.UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields,
		nsxv1alpha.UnsupportedExpressionReasonUnsupportedPathExpressionFields,
		nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression:
		return true
	default:
		return false
	}
}
