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

package ir

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"database/sql"
	"database/sql/driver"
	"github.com/beevik/etree"
	"github.com/npiganeau/yep/yep/tools"
)

type ViewType string

const (
	VIEW_TYPE_TREE     ViewType = "tree"
	VIEW_TYPE_LIST     ViewType = "list"
	VIEW_TYPE_FORM     ViewType = "form"
	VIEW_TYPE_GRAPH    ViewType = "graph"
	VIEW_TYPE_CALENDAR ViewType = "calendar"
	VIEW_TYPE_DIAGRAM  ViewType = "diagram"
	VIEW_TYPE_GANTT    ViewType = "gantt"
	VIEW_TYPE_KANBAN   ViewType = "kanban"
	VIEW_TYPE_SEARCH   ViewType = "search"
	VIEW_TYPE_QWEB     ViewType = "qweb"
)

type ViewInheritanceMode string

const (
	VIEW_PRIMARY   ViewInheritanceMode = "primary"
	VIEW_EXTENSION ViewInheritanceMode = "extension"
)

var ViewsRegistry *ViewsCollection

func MakeViewRef(id string) ViewRef {
	view := ViewsRegistry.GetViewById(id)
	if view == nil {
		return ViewRef{}
	}
	return ViewRef{id, view.Name}
}

type ViewRef [2]string

func (e *ViewRef) String() string {
	sl := []string{e[0], e[1]}
	return fmt.Sprintf(`[%s]`, strings.Join(sl, ","))
}

// Value extracts ID of our ViewRef for storing in the database.
func (vr ViewRef) Value() (driver.Value, error) {
	return driver.Value(vr[0]), nil
}

// Scan fetches the name of our view from the ID
// stored in the database to fill the ViewRef.
func (vr *ViewRef) Scan(src interface{}) error {
	var source string
	switch s := src.(type) {
	case string:
		source = s
	case []byte:
		source = string(s)
	default:
		return fmt.Errorf("Invalid type for ViewRef: %T", src)
	}
	*vr = MakeViewRef(source)
	return nil
}

var _ driver.Valuer = ActionRef{}
var _ sql.Scanner = &ActionRef{}

type ViewsCollection struct {
	sync.RWMutex
	views        map[string]*View
	orderedViews map[string][]*View
}

// NewViewCollection returns a pointer to a new
// ViewsCollection instance
func NewViewsCollection() *ViewsCollection {
	res := ViewsCollection{
		views:        make(map[string]*View),
		orderedViews: make(map[string][]*View),
	}
	return &res
}

// AddView adds the given view to our ViewsCollection
func (vc *ViewsCollection) AddView(v *View) {
	vc.Lock()
	var index int8
	for i, view := range vc.orderedViews[v.Model] {
		index = int8(i)
		if view.Priority >= v.Priority {
			break
		}
	}
	defer vc.Unlock()
	vc.views[v.ID] = v
	endElems := make([]*View, len(vc.orderedViews[v.Model][index:]))
	copy(endElems, vc.orderedViews[v.Model][index:])
	vc.orderedViews[v.Model] = append(append(vc.orderedViews[v.Model][:index], v), endElems...)
}

// GetViewById returns the View with the given id
func (vc *ViewsCollection) GetViewById(id string) *View {
	return vc.views[id]
}

/*
GetFirstViewForModel returns the first view of type viewType for the given model
*/
func (vc *ViewsCollection) GetFirstViewForModel(model string, viewType ViewType) *View {
	for _, view := range vc.orderedViews[model] {
		if view.Type == viewType {
			return view
		}
	}
	tools.LogAndPanic(log, "No view of this type in model", "type", viewType, "model", model)
	return nil
}

type View struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Model              string              `json:"model"`
	Type               ViewType            `json:"type"`
	Priority           uint8               `json:"priority"`
	Arch               string              `json:"arch"`
	InheritID          *View               `json:"inherit_id"`
	InheritChildrenIDs []*View             `json:"inherit_children_ids"`
	FieldParent        string              `json:"field_parent"`
	InheritanceMode    ViewInheritanceMode `json:"mode"`
	Fields             []string
	//GroupsID []*Group
}

/*
LoadViewFromEtree reads the view given etree.Element, creates or updates the view
and adds it to the view registry if it not already.
*/
func LoadViewFromEtree(element *etree.Element) {
	// We populate a viewHash from XML data fields
	viewHash := make(map[string]interface{})
	viewHash["id"] = element.SelectAttrValue("id", "NO_ID")
	for _, fieldNode := range element.FindElements("field") {
		name := fieldNode.SelectAttrValue("name", "NO_NAME")
		if len(fieldNode.ChildElements()) > 0 {
			fieldType := fieldNode.SelectAttrValue("type", "text")
			switch fieldType {
			case "xml":
				nodeDoc := etree.NewDocument()
				nodeDoc.SetRoot(fieldNode.ChildElements()[0].Copy())
				value, _ := nodeDoc.WriteToString()
				viewHash[name] = value
			default:
				tools.LogAndPanic(log, "Unknown field type", "type", fieldType, "view", viewHash["id"])
			}
		} else {
			viewHash[name] = fieldNode.Text()
		}
	}
	// We marshal viewHash in JSON and then unmarshal into a View struct
	bytes, _ := json.Marshal(viewHash)
	var view View
	if err := json.Unmarshal(bytes, &view); err != nil {
		tools.LogAndPanic(log, "Unable to unmarshal view", "viewHash", viewHash, "error", err)
	}
	ViewsRegistry.AddView(&view)
}
