/*
Copyright 2024 The Flux authors

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

package gogit

import (
	"time"

	"github.com/fluxcd/pkg/cache"
	"github.com/fluxcd/pkg/git"
)

// cacheObject sets the object and its expiration info in the cache
func cacheObject[T git.Credentials](store cache.Expirable[cache.StoreObject[T]], creds T, key string, expiresAt time.Time) error {
	obj := cache.StoreObject[T]{
		Object: creds,
		Key:    key,
	}

	err := store.Set(obj)
	if err != nil {
		return err
	}

	return store.SetExpiration(obj, expiresAt)
}

// getObjectFromCache uses the key to get the object from the cache
func getObjectFromCache[T git.Credentials](store cache.Expirable[cache.StoreObject[T]], key string) (T, bool, error) {
	val, exists, err := store.GetByKey(key)
	return val.Object, exists, err
}

// invalidateObjectInCache deletes the entry from the cache
func invalidateObjectInCache[T git.Credentials](store cache.Expirable[cache.StoreObject[T]], creds T, key string) error {
	obj := cache.StoreObject[T]{
		Object: creds,
		Key:    key,
	}
	return store.Delete(obj)
}
