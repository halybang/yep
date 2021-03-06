// Copyright 2016 NDP Systèmes. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tools

import (
	"fmt"
	"io/ioutil"
)

// ListStaticFiles get all file names of the static files that are in
// the "server/static/*/<subDir>" directories.
func ListStaticFiles(subDir string, modules []string) []string {
	var res []string
	for _, module := range modules {
		dirName := fmt.Sprintf("yep/server/static/%s/%s", module, subDir)
		fileInfos, _ := ioutil.ReadDir(dirName)
		for _, fi := range fileInfos {
			if !fi.IsDir() {
				res = append(res, fmt.Sprintf("%s/%s", dirName, fi.Name()))
			}
		}
	}
	return res
}
