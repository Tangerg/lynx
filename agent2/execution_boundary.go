package agent2

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func failureFromError(kind FailureKind, code string, err error) (Failure, error) {
	message := "unknown error"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	if message == "" {
		message = "unknown error"
	}
	if len(message) > maxFailureMessageBytes {
		message = message[:maxFailureMessageBytes]
	}
	return NewFailure(kind, code, message)
}

func failureKindForError(err error) FailureKind {
	var panicError executionPanic
	if errors.As(err, &panicError) {
		return FailureKindPanic
	}
	return FailureKindExecution
}

type executionPanic struct{ value any }

func (panicError executionPanic) Error() string {
	return fmt.Sprintf("execution panicked: %v", panicError.value)
}

func startExecution(definition Definition, input Input) (execution Execution, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = nil
			err = executionPanic{value: recovered}
		}
	}()
	execution, err = definition.Start(input)
	if err == nil && nilInterface(execution) {
		return nil, errors.New("definition.Start returned nil execution")
	}
	return execution, err
}

func restoreExecution(definition Definition, state ExecutionState) (execution Execution, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = nil
			err = executionPanic{value: recovered}
		}
	}()
	execution, err = definition.Restore(state)
	if err == nil && nilInterface(execution) {
		return nil, errors.New("definition.Restore returned nil execution")
	}
	return execution, err
}

func stepExecution(ctx context.Context, execution Execution, signals []Signal) (transition Transition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			transition = Transition{}
			err = executionPanic{value: recovered}
		}
	}()
	return execution.Step(ctx, signals)
}

func captureExecution(execution Execution) (state ExecutionState, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			state = ExecutionState{}
			err = executionPanic{value: recovered}
		}
	}()
	state, err = execution.Snapshot()
	if err == nil && !state.Valid() {
		return ExecutionState{}, ErrInvalidExecutionState
	}
	return state, err
}

func initializeExecution(
	definition Definition,
	input Input,
) (Execution, ExecutionState, Failure, error) {
	execution, err := startExecution(definition, input)
	if err != nil {
		failure := processInitializationFailure(
			failureKindForError(err), "engine.process.start.failed", err,
		)
		return nil, ExecutionState{}, failure, fmt.Errorf("start Execution: %w", err)
	}
	state, err := captureExecution(execution)
	if err != nil {
		failure := processInitializationFailure(
			failureKindForError(err), "engine.process.snapshot.failed", err,
		)
		return nil, ExecutionState{}, failure, fmt.Errorf("capture initial Execution state: %w", err)
	}
	restored, err := restoreExecution(definition, state)
	if err != nil {
		failure := processInitializationFailure(
			failureKindForError(err), "engine.process.snapshot.unrestorable", err,
		)
		return nil, ExecutionState{}, failure, fmt.Errorf("validate initial Execution state: %w", err)
	}
	return restored, state, Failure{}, nil
}

func processInitializationFailure(kind FailureKind, code string, cause error) Failure {
	failure, err := failureFromError(kind, code, cause)
	if err == nil {
		return failure
	}
	failure, _ = NewFailure(
		FailureKindContract,
		"engine.process.start_outcome.invalid",
		"invalid Process initialization failure",
	)
	return failure
}

func acknowledgePreparedStep(
	acknowledger PreparedStepAcknowledger,
	ctx context.Context,
	snapshot Snapshot,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("prepared-step acknowledger panicked: %v", recovered)
		}
	}()
	return acknowledger.AcknowledgePreparedStep(ctx, snapshot)
}
