package workflowy

import (
	"net/http"
	"net/url"
	"fmt"
	"errors"
	"strings"
	"encoding/base32"
	"crypto/rand"
	"io/ioutil"
	"github.com/Jeffail/gabs"
	"html"
)

type WorkflowyClient struct {
	Session                 string
	client                  string
	most_recent_transaction string
	owner                   int
	json                    *gabs.Container
	pending_operations      [](*gabs.Container)
}

type WorkflowyItem struct {
	Id             string
	Name           string
	Completed      bool
	Priority       int
	Description    *string
	Children_names []string
}

func GetSession(username string, password string) (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)
	resp, err := client.PostForm("https://workflowy.com/accounts/login/", form)
	if (resp.StatusCode != 302) {
		return "", err
	}
	if err != nil {
		return "", err
	} else {
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "sessionid" {
				return cookie.Value, nil
			}
		}
	}
	return "", errors.New("Unknown")
}

func NewClient(session string) (*WorkflowyClient, error) {
	workflowyClient := &WorkflowyClient{Session: session, pending_operations: [](*gabs.Container){}}
	err := workflowyClient.refreshData()
	return workflowyClient, err
}

func (client *WorkflowyClient) LookupItem(path ... string) (WorkflowyItem, error) {
	if client.json.ExistsP("projectTreeData.mainProjectTreeInfo.rootProjectChildren") {
		return lookupProjectNode(client.json.Path("projectTreeData.mainProjectTreeInfo.rootProjectChildren"), path)
	} else {
		return WorkflowyItem{}, errors.New("Malformed workflowy json")
	}
}

func (client *WorkflowyClient) AddCreate(name string, priority int, parent *string, description *string) {
	item_id := makeItemId()
	parentString := "None"
	if parent != nil {
		parentString = *parent
	}
	op := gabs.New()
	op.Set("create", "type")
	op.Set(item_id, "data", "projectid")
	op.Set(parentString, "data", "parentid")
	op.Set(priority, "data", "priority")
	client.pending_operations = append(client.pending_operations, op)
	client.AddUpdate(item_id, &name, &priority, parent, description)
}

func (client *WorkflowyClient) AddUpdate(item_id string, name *string, priority *int, parent *string, description *string) {
	op := gabs.New()
	op.Set("edit", "type")
	op.Set(item_id, "data", "projectid")
	if name != nil {
		op.Set(html.EscapeString(*name), "data", "name")
	}
	if description != nil {
		op.Set(html.EscapeString(*description), "data", "description")
	}
	if parent != nil {
		op.Set(*parent, "data", "parentid")
	}
	if priority != nil {
		op.Set(*priority, "data", "priority")
	}
	client.pending_operations = append(client.pending_operations, op)
}

func (client *WorkflowyClient) AddDelete(item_id string) {
	op := gabs.New()
	op.Set("delete", "type")
	op.Set(item_id, "data", "projectid")
	client.pending_operations = append(client.pending_operations, op)
}

func (client *WorkflowyClient) AddComplete(item_id string) {
	op := gabs.New()
	op.Set("complete", "type")
	op.Set(item_id, "data", "projectid")
	client.pending_operations = append(client.pending_operations, op)
}

func (client *WorkflowyClient) AddUncomplete(item_id string) {
	op := gabs.New()
	op.Set("uncomplete", "type")
	op.Set(item_id, "data", "projectid")
	client.pending_operations = append(client.pending_operations, op)
}

func (client *WorkflowyClient) ApplyAndRefresh() error {
	form := url.Values{}
	operationList := newOperationList(client.most_recent_transaction, client.pending_operations)
	form.Add("client_id", client.client)
	form.Add("crosscheck_user_id", string(client.owner))
	form.Add("push_poll_id", makeUpdateId())
	form.Add("client_version", "18")
	form.Add("push_poll_data", operationList.String())
	req, _ := http.NewRequest("POST", "https://workflowy.com/push_and_poll", strings.NewReader(form.Encode()))
	req.Header.Add("Cookie", "sessionid="+client.Session)
	_, err := http.DefaultClient.Do(req)
	if (err != nil) {
		return err
	}
	client.pending_operations = [](*gabs.Container){}
	client.refreshData()
	return nil
}

func (client *WorkflowyClient) refreshData() error {
	req, err := http.NewRequest("GET", "https://workflowy.com/get_initialization_data?client_version=18", nil)
	req.Header.Add("Cookie", "sessionid="+client.Session)
	resp, err := http.DefaultClient.Do(req)
	if (err != nil) {
		return err
	} else {
		readall, _ := ioutil.ReadAll(resp.Body)
		json, _ := gabs.ParseJSON(readall)
		client.client = json.Path("projectTreeData.clientId").Data().(string)
		client.owner = int(json.Path("projectTreeData.mainProjectTreeInfo.ownerId").Data().(float64))
		client.most_recent_transaction = json.Path("projectTreeData.mainProjectTreeInfo.initialMostRecentOperationTransactionId").Data().(string)
		client.json = json
		return nil
	}
}

func lookupProjectNode(json *gabs.Container, path []string) (WorkflowyItem, error) {
	children, err := json.Children()
	if err != nil {
		return WorkflowyItem{}, errors.New("could not find children")
	}

	for priority, child := range children {
		childName := child.Path("nm").Data().(string)
		if childName == path[0] {
			if len(path) == 1 {
				item_id := child.Path("id").Data().(string)
				name := child.Path("nm").Data().(string)
				completed := child.Exists("cp")
				children_names := []string{}

				// if this item has children, gather their names
				if child.Exists("ch") {
					metaChildren, _ := child.Path("ch").Children()
					for _, metaChild := range metaChildren {
						children_names = append(children_names, metaChild.Path("nm").Data().(string))
					}
				}

				var description *string = nil
				if child.Exists("no") {
					descriptionString := child.Path("no").Data().(string)
					description = &descriptionString
				}
				return WorkflowyItem{
					Id:             item_id,
					Name:           name,
					Priority:       priority,
					Children_names: children_names,
					Completed:      completed,
					Description:    description,
				}, nil

			} else {
				return lookupProjectNode(child.Path("ch"), path[1:])
			}
		}
	}
	return WorkflowyItem{}, errors.New("Could not find node " + path[0])
}

func newOperationList(lasttxn string, operations [](*gabs.Container)) *gabs.Container {
	jsons := gabs.New()
	jsons.Set(lasttxn, "most_recent_operation_transaction_id")
	jsons.Array("operations")
	for _, operation := range operations {
		jsons.ArrayAppend(operation.Data(), "operations")
	}
	containingArray := gabs.New()
	containingArray.Array()
	containingArray.ArrayAppend(jsons.Data())
	return containingArray
}

func makeItemId() (uuid string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	uuid = strings.ToLower(fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
	return
}

func makeUpdateId() string {
	b := make([]byte, 64)
	rand.Read(b)
	return base32.StdEncoding.EncodeToString(b)[0:7]
}
