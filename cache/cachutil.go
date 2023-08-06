package cache

import (
	"reflect"
	"time"
)

var globalCache Cache

func CacheInit() {

	var err error
	globalCache, err = New(
		WithSizeInMB(10),
		WithTTL(12*time.Hour),
		WithCleanFrequency(24*time.Hour),
	)
	if err != nil {
		panic("Failed to initialize global cache: " + err.Error())
	}
}

func GetCache() Cache {
	return globalCache
}

func CachedFunction(fn interface{}, key string) func(...interface{}) (interface{}, error) {
	return func(args ...interface{}) (interface{}, error) {

		// Check if the result is already cached
		if cachedResult, err := globalCache.Get(key); err == nil {
			return cachedResult, nil
		}

		// If not cached, call the original function to get the result
		f := reflect.ValueOf(fn)
		if len(args) == 0 {
			args = nil // Ensure nil is passed when there are no arguments.
		}
		result := f.Call(getArgs(args))

		// Check if the function returned an error as the last return value
		if err, ok := result[len(result)-1].Interface().(error); ok && err != nil {
			return nil, err
		}

		// Extract the result from the function call
		finalResult := result[0].Interface()

		// Cache the result with a default TTL (time-to-live)
		globalCache.Set(key, finalResult)

		return finalResult, nil
	}
}

func getArgs(args []interface{}) []reflect.Value {
	var values []reflect.Value
	for _, arg := range args {
		values = append(values, reflect.ValueOf(arg))
	}
	return values
}
