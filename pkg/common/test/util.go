/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Contains utilities that will be shared across unit tests.

package test

import (
	"errors"
	"reflect"
	"runtime"
	"testing"
)

// This will return the string name of the method that called this function
func CallerfunctionName() string {
	pc, _, _, _ := runtime.Caller(1)
	return runtime.FuncForPC(pc).Name()
}

// This will return the string name of the method
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

/*
	This will test if the function currently executing is in the right sequence in a list of execution order.
	executionsequenceList - Array of function names that will represent the execution order
	executingFunction - The m
*/
func CheckExecutionSequence(executionsequenceList []string, executingFunction string, index *int, t *testing.T) {
	if executionsequenceList != nil {
		if sequenceListCount := len(executionsequenceList); sequenceListCount > 0 {
			if *index >= sequenceListCount {
				t.Errorf("Sequence #%d is out of bounds for function %s!", *index, executingFunction)
			} else if executionsequenceList[*index] != executingFunction {
				t.Errorf("Executing the wrong function (%s) when it should have been %s!", executingFunction, executionsequenceList[*index])
			}
			*index += 1
		}
	}
}

/*
	This will trigger error return for the toolSet functions to test error condition. Parameters are:
	functionWithError - The function that you want error to happen
	executingFunction - This is the function currently running. This will be supplied by using callerfunctionName()
	functionWithErrorNumber - For functions that are called multiple times, this is the instance # of the function that
		the error will be triggered. For example if you specify 2, then the 2nd call to the function is the one that
		will be processed
	functionWithErrorCount - This is just a counter of how many times the target function has been called. This will be
		used by functionWithErrorNumber to identify if it has hit that # of execution of the function to trigger the
		return of the error.
*/
func CheckIfFunctionHasError(functionWithError string, executingFunction string, functionWithErrorNumber int, functionWithErrorCount *int, t *testing.T) error {
	if functionWithError == executingFunction {
		*functionWithErrorCount++
		if functionWithErrorNumber == *functionWithErrorCount {
			return errors.New("Error is triggered!")
		}
	}
	return nil
}

func VerifyExecutionSequenceCompleted(executionSequenceList []string, executionIndex int, t *testing.T) {
	totalFunctionsInExecutionList := len(executionSequenceList)
	if executionIndex != totalFunctionsInExecutionList {
		t.Errorf("Function execution list has a total of %d when it should have been %d!", executionIndex, totalFunctionsInExecutionList)
	}
}
