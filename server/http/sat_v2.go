package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"reflect"
	"strconv"
)

//Sat state
type Sat struct {
	Config  SatConfig     `json:"config" yaml:"config"`
	Current SatCurrent    `json:"current" yaml:"current"`
	Items   []SatItem     `json:"items" yaml:"items"`
	Labels  LabelMap      `json:"labels" yaml:"labels"`
	Tracks  TrackMap      `json:"tracks" yaml:"tracks"`
	Shapes  ShapeMap      `json:"shapes" yaml:"shapes"`
	Actions []interface{} `json:"actions" yaml:"actions"`
}

type LabelMap map[int]SatLabel

func (labels *LabelMap) UnmarshalJSON(data []byte) error {
	(*labels) = make(map[int]SatLabel)
	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		label := SatLabel{}
		mapstructure.Decode(v, &label)
		i, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		(*labels)[i] = label
	}
	return nil
}

type ShapeMap map[int]interface{}

func (shapes *ShapeMap) UnmarshalJSON(data []byte) error {
	(*shapes) = make(map[int]interface{})
	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		i, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		(*shapes)[i] = v
	}
	return nil
}

type TrackMap map[int][]SatLabel

func (tracks *TrackMap) UnmarshalJSON(data []byte) error {
	(*tracks) = make(map[int][]SatLabel)
	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		var labels []SatLabel
		mapstructure.Decode(v, &labels)
		i, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		(*tracks)[i] = labels
	}
	return nil
}

func (sat *Sat) GetKey() string {
	return path.Join(sat.Config.ProjectName, "submissions", sat.Config.TaskId,
		sat.Config.WorkerId, strconv.FormatInt(sat.Config.SubmitTime, 10))
}

func (sat *Sat) GetFields() map[string]interface{} {
	return map[string]interface{}{
		"config":  sat.Config,
		"current": sat.Current,
		"items":   sat.Items,
		"labels":  sat.Labels,
		"tracks":  sat.Tracks,
		"shapes":  sat.Shapes,
		"actions": sat.Actions,
	}
}

//current state of Sat
type SatCurrent struct {
	Item        int `json:"item" yaml:"item"`
	Label       int `json:"label" yaml:"label"`
	MaxObjectId int `json:"maxObjectId" yaml:"maxObjectId"`
}

//Sat configuration state
type SatConfig struct {
	AssignmentId    string      `json:"assignmentId" yaml:"assignmentId"`
	ProjectName     string      `json:"projectName" yaml:"projectName"`
	ItemType        string      `json:"itemType" yaml:"itemType"`
	LabelType       string      `json:"labelType" yaml:"labelType"`
	TaskSize        int         `json:"taskSize" yaml:"taskSize"`
	HandlerUrl      string      `json:"handlerUrl" yaml:"handlerUrl"`
	PageTitle       string      `json:"pageTitle" yaml:"pageTitle"`
	InstructionPage string      `json:"instructionPage" yaml:"instructionPage"`
	DemoMode        bool        `json:"demoMode" yaml:"demoMode"`
	BundleFile      string      `json:"bundleFile" yaml:"bundleFile"`
	Categories      []string    `json:"categories" yaml:"categories"`
	Attributes      []Attribute `json:"attributes" yaml:"attributes"`
	TaskId          string      `json:"taskId" yaml:"taskId"`
	WorkerId        string      `json:"workerId" yaml:"workerId"`
	StartTime       int64       `json:"startTime" yaml:"startTime"`
	SubmitTime      int64       `json:"submitTime" yaml:"submitTime"`
}

type SatItem struct {
	Id     int    `json:"id" yaml:"id"`
	Index  int    `json:"index" yaml:"index"`
	Url    string `json:"url" yaml:"url"`
	Active bool   `json:"active" yaml:"active"`
	Loaded bool   `json:"loaded" yaml:"loaded"`
	Labels []int  `json:"labels, []int" yaml:"labels"`
}

type SatLabel struct {
	Id            int              `json:"id" yaml:"id"`
	Item          int              `json:"item" yaml:"item"`
	Category      []int            `json:"category" yaml:"category"`
	Attributes    map[string][]int `json:"attributes" yaml:"attributes"`
	Parent        int              `json:"parent" yaml:"parent"`
	Children      []int            `json:"children" yaml:"children"`
	NumChildren   int              `json:"numChildren" yaml:"numChildren"`
	Valid         bool             `json:"valid" yaml:"valid"`
	Shapes        []int            `json:"shapes" yaml:"shapes"`
	SelectedShape int              `json:"selectedShape" yaml:"selectedShape"`
	State         int              `json:"state" yaml:"state"`
}

// Get the most recent assignment given the needed fields.
func GetSat(projectName string, taskIndex string, workerId string) (Sat, error) {
	sat := Sat{}
	submissionsPath := path.Join(projectName, "submissions", taskIndex, workerId)
	keys := storage.ListKeys(submissionsPath)
	// if any submissions exist, get the most recent one
	if len(keys) > 0 {
		Info.Printf("Reading %s\n", keys[len(keys)-1])
		fields, err := storage.Load(keys[len(keys)-1])
		if err != nil {
			return Sat{}, err
		}
		loadedSatJson, err := json.Marshal(fields)
		if err != nil {
			return Sat{}, err
		}
		if err := json.Unmarshal(loadedSatJson, &sat); err != nil {
			return Sat{}, err
		}
	} else {
		var assignment Assignment
		assignmentPath := path.Join(projectName, "assignments", taskIndex, workerId)
		Info.Printf("Reading %s\n", assignmentPath)
		fields, err := storage.Load(assignmentPath)
		if err != nil {
			return Sat{}, err
		}
		mapstructure.Decode(fields, &assignment)
		sat = assignmentToSat(&assignment)
	}
	return sat, nil
}

func GetAssignmentV2(projectName string, taskIndex string, workerId string) (Assignment, error) {
	assignment := Assignment{}
	assignmentPath := path.Join(projectName, "assignments", taskIndex, workerId)
	fields, err := storage.Load(assignmentPath)
	if err != nil {
		return Assignment{}, err
	}
	mapstructure.Decode(fields, &assignment)
	return assignment, nil
}

// Handles the loading of an assignment given its project name, task index, and worker ID.
func postLoadAssignmentV2Handler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		Error.Println(err)
	}
	assignmentToLoad := Assignment{}
	err = json.Unmarshal(body, &assignmentToLoad)
	if err != nil {
		Error.Println(err)
	}
	projectName := assignmentToLoad.Task.ProjectOptions.Name
	taskIndex := Index2str(assignmentToLoad.Task.Index)
	var loadedAssignment Assignment
	var loadedSat Sat
	if !storage.HasKey(path.Join(projectName, "assignments",
		taskIndex, DEFAULT_WORKER)) {
		// if assignment does not exist, create it
		loadedAssignment, err = CreateAssignment(projectName, taskIndex,
			DEFAULT_WORKER)
		if err != nil {
			Error.Println(err)
			return
		}
		loadedSat = assignmentToSat(&loadedAssignment)
	} else {
		loadedSat, err = GetSat(projectName, taskIndex,
			DEFAULT_WORKER)
		if err != nil {
			Error.Println(err)
			return
		}
	}
	loadedSat.Config.StartTime = recordTimestamp()
	loadedSatJson, err := json.Marshal(loadedSat)
	if err != nil {
		Error.Println(err)
	}
	w.Write(loadedSatJson)
}

func executeLabelingTemplateV2(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	// get task name from the URL
	projectName := r.URL.Query()["project_name"][0]
	taskIndex, _ := strconv.ParseInt(r.URL.Query()["task_index"][0], 10, 32)
	if !storage.HasKey(path.Join(projectName, "assignments",
		Index2str(int(taskIndex)), DEFAULT_WORKER)) {
		// if assignment does not exist, create it
		assignment, err := CreateAssignment(projectName, Index2str(int(taskIndex)), DEFAULT_WORKER)
		if err != nil {
			Error.Println(err)
			return
		}
		tmpl.Execute(w, assignment)
	} else {
		// otherwise, get that assignment
		assignment, err := GetAssignmentV2(projectName, Index2str(int(taskIndex)), DEFAULT_WORKER)
		if err != nil {
			Error.Println(err)
			return
		}
		tmpl.Execute(w, assignment)
	}
}

func Label2dv2Handler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(env.Label2dPath(r.FormValue("v")))
	if err != nil {
		Error.Println(err)
	}
	executeLabelingTemplateV2(w, r, tmpl)
}

// Essentially rewriting the decodeBaseJson logic, need to get rid of this
// when backend is completely transferred to redux
func assignmentToSat(assignment *Assignment) Sat {
	var categories []string
	for _, category := range assignment.Task.ProjectOptions.Categories {
		categories = append(categories, category.Name)
	}
	var items []SatItem
	for _, item := range assignment.Task.Items {
		satItem := SatItem{
			Id:     item.Index,
			Index:  item.Index,
			Url:    item.Url,
			Labels: []int{},
		}
		items = append(items, satItem)
	}
	// only items are needed because this function is only called once
	// at the first visit to annotation interface before submission
	// and will go away when redux have its own project creation logic
	labels := map[int]SatLabel{}
	tracks := map[int][]SatLabel{}
	shapes := map[int]interface{}{}
	projectOptions := assignment.Task.ProjectOptions
	loadedSatConfig := SatConfig{
		AssignmentId:    assignment.Id,
		ProjectName:     projectOptions.Name,
		ItemType:        projectOptions.ItemType,
		LabelType:       projectOptions.LabelType,
		TaskSize:        projectOptions.TaskSize,
		HandlerUrl:      projectOptions.HandlerUrl,
		PageTitle:       projectOptions.PageTitle,
		InstructionPage: projectOptions.Instructions,
		DemoMode:        projectOptions.DemoMode,
		BundleFile:      projectOptions.BundleFile,
		Categories:      categories,
		Attributes:      projectOptions.Attributes,
		TaskId:          Index2str(assignment.Task.Index),
		WorkerId:        assignment.WorkerId,
		StartTime:       assignment.StartTime,
		SubmitTime:      assignment.SubmitTime,
	}
	satCurrent := SatCurrent{
		Item:        -1,
		Label:       -1,
		MaxObjectId: -1,
	}
	loadedSat := Sat{
		Config:  loadedSatConfig,
		Current: satCurrent,
		Items:   items,
		Labels:  labels,
		Tracks:  tracks,
		Shapes:  shapes,
		Actions: []interface{}{},
	}
	return loadedSat
}

func postSaveV2Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		Error.Println(err)
	}
	// var fields map[string]interface{}
	// err = json.Unmarshal(body, &fields)
	// if err != nil {
	// 	   Error.Println(err)
	// }
	// assignment := Sat{}
	// mapstructure.Decode(fields, &assignment)
	assignment := Sat{}
	err = json.Unmarshal(body, &assignment)
	if err != nil {
		Error.Println(err)
	}
	if assignment.Config.DemoMode {
		Error.Println(errors.New("can't save a demo project"))
		w.Write(nil)
		return
	}
	// TODO: don't send all events to front end, and append these events to most recent
	assignment.Config.SubmitTime = recordTimestamp()
	err = storage.Save(assignment.GetKey(), assignment.GetFields())
	if err != nil {
		Error.Println(err)
	}
	w.Write(nil)
}

// Handles the export of submitted assignments
func postExportV2Handler(w http.ResponseWriter, r *http.Request) {
	var projectName = r.FormValue("project_name")
	key := path.Join(projectName, "project")
	fields, err := storage.Load(key)
	if err != nil {
		Error.Println(err)
	}
	projectToLoad := Project{}
	mapstructure.Decode(fields, &projectToLoad)

	// Grab the latest submissions from all tasks
	tasks, err := GetTasksInProject(projectName)
	if err != nil {
		Error.Println(err)
		return
	}
	items := []ItemExport{}
	for _, task := range tasks {
		sat, err := GetSat(projectName, Index2str(task.Index), DEFAULT_WORKER)
		if err == nil {
			for _, itemToLoad := range sat.Items {
				item := ItemExport{}
				item.Index = itemToLoad.Index
				if projectToLoad.Options.ItemType == "video" {
					item.VideoName = projectToLoad.Options.Name + "_" + Index2str(task.Index)
				}
				item.Timestamp = 10000 // to be fixed
				item.Name = itemToLoad.Url
				item.Url = itemToLoad.Url
				item.Attributes = map[string]string{}
				if len(itemToLoad.Labels) > 0 {
					itemLabel := sat.Labels[itemToLoad.Labels[0]]
					keys := reflect.ValueOf(itemLabel.Attributes).MapKeys()
					strkeys := make([]string, len(keys))
					for i := 0; i < len(keys); i++ {
						strkeys[i] = keys[i].String()
					}
					for _, key := range strkeys {
						for _, attribute := range sat.Config.Attributes {
							if attribute.Name == key {
								item.Attributes[key] = attribute.Values[itemLabel.Attributes[key][0]]
								break
							}
						}
					}
				}
				items = append(items, item)
			}
		} else {
			// if file not found, return list of items with url
			Info.Println(err)
			for _, itemToLoad := range task.Items {
				item := ItemExport{}
				item.Index = itemToLoad.Index
				if projectToLoad.Options.ItemType == "video" {
					item.VideoName = projectToLoad.Options.Name + "_" + Index2str(task.Index)
				}
				item.Timestamp = 10000 // to be fixed
				item.Name = itemToLoad.Url
				item.Url = itemToLoad.Url
				items = append(items, item)
			}
		}
	}

	exportJson, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		Error.Println(err)
	}

	//set relevant header.
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%s_results.json", projectName))
	io.Copy(w, bytes.NewReader(exportJson))
}
