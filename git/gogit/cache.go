/*
Copyright 2021 The Flux authors

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

// cacheCredential sets the credential and its expiration info in the cache
func cacheCredential[T *git.Credentials](store cache.Expirable[T], creds T, key string, expiresAt time.Time) error {
	err := store.Set(key, creds)
	if err != nil {
		return err
	}

	return store.SetExpiration(key, expiresAt)
}

// getCredentialFromCache uses the key to get the credential from the cache
func getCredentialFromCache[T *git.Credentials](store cache.Expirable[T], key string) (T, error) {
	val, err := store.Get(key)
	if err != nil {
		if err == cache.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return *val, nil
}

// invalidateCredentialInCache deletes the credential entry from the cache
func invalidateCredentialInCache[T *git.Credentials](store cache.Expirable[T], key string) error {
	return store.Delete(key)
}
