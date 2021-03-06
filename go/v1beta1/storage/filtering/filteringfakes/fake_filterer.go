// Code generated by counterfeiter. DO NOT EDIT.
package filteringfakes

import (
	"sync"

	"github.com/rode/grafeas-elasticsearch/go/v1beta1/storage/filtering"
)

type FakeFilterer struct {
	ParseExpressionStub        func(string) (*filtering.Query, error)
	parseExpressionMutex       sync.RWMutex
	parseExpressionArgsForCall []struct {
		arg1 string
	}
	parseExpressionReturns struct {
		result1 *filtering.Query
		result2 error
	}
	parseExpressionReturnsOnCall map[int]struct {
		result1 *filtering.Query
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeFilterer) ParseExpression(arg1 string) (*filtering.Query, error) {
	fake.parseExpressionMutex.Lock()
	ret, specificReturn := fake.parseExpressionReturnsOnCall[len(fake.parseExpressionArgsForCall)]
	fake.parseExpressionArgsForCall = append(fake.parseExpressionArgsForCall, struct {
		arg1 string
	}{arg1})
	stub := fake.ParseExpressionStub
	fakeReturns := fake.parseExpressionReturns
	fake.recordInvocation("ParseExpression", []interface{}{arg1})
	fake.parseExpressionMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *FakeFilterer) ParseExpressionCallCount() int {
	fake.parseExpressionMutex.RLock()
	defer fake.parseExpressionMutex.RUnlock()
	return len(fake.parseExpressionArgsForCall)
}

func (fake *FakeFilterer) ParseExpressionCalls(stub func(string) (*filtering.Query, error)) {
	fake.parseExpressionMutex.Lock()
	defer fake.parseExpressionMutex.Unlock()
	fake.ParseExpressionStub = stub
}

func (fake *FakeFilterer) ParseExpressionArgsForCall(i int) string {
	fake.parseExpressionMutex.RLock()
	defer fake.parseExpressionMutex.RUnlock()
	argsForCall := fake.parseExpressionArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeFilterer) ParseExpressionReturns(result1 *filtering.Query, result2 error) {
	fake.parseExpressionMutex.Lock()
	defer fake.parseExpressionMutex.Unlock()
	fake.ParseExpressionStub = nil
	fake.parseExpressionReturns = struct {
		result1 *filtering.Query
		result2 error
	}{result1, result2}
}

func (fake *FakeFilterer) ParseExpressionReturnsOnCall(i int, result1 *filtering.Query, result2 error) {
	fake.parseExpressionMutex.Lock()
	defer fake.parseExpressionMutex.Unlock()
	fake.ParseExpressionStub = nil
	if fake.parseExpressionReturnsOnCall == nil {
		fake.parseExpressionReturnsOnCall = make(map[int]struct {
			result1 *filtering.Query
			result2 error
		})
	}
	fake.parseExpressionReturnsOnCall[i] = struct {
		result1 *filtering.Query
		result2 error
	}{result1, result2}
}

func (fake *FakeFilterer) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.parseExpressionMutex.RLock()
	defer fake.parseExpressionMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeFilterer) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ filtering.Filterer = new(FakeFilterer)
