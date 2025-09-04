# Guide: Adding a New Tool Type to Dialogr

*A comprehensive guide based on lessons learned from implementing the alertmanager tool (using Alertmanager v2 API)*

## Overview
This guide walks through the complete process of adding a new tool type to Dialogr, ensuring proper integration with both backend and frontend systems while maintaining the build green.

## Prerequisites
- Understanding of Go, HTMX, and Templ
- Familiarity with Dialogr's architecture
- Access to the codebase with proper development environment

## Step-by-Step Implementation

### Phase 1: Backend Implementation

#### 1.1 Create the Tool Implementation File
**File**: `backend/tool/{toolname}.go`

```go
package tool

import (
    // Required imports
    "encoding/json"
    "fmt"
    "github.com/riclib/dialogr/internal/models"
    "github.com/sashabaranov/go-openai/jsonschema"
)

// Define parameter struct for the tool
type {ToolName}ToolParams struct {
    Action string `json:"action" description:"Primary action to perform"`
    // Add other parameters as needed
}

// Define JSON schema for parameters
var {ToolName}ToolParamsJsonSchema = jsonschema.Definition{
    Type: "object",
    Properties: map[string]jsonschema.Definition{
        "action": {
            Type:        "string",
            Description: "Action description",
            Enum:        []string{"action1", "action2"}, // Define valid actions
        },
        // Add other properties
    },
    Required: []string{"action"},
}

// Main tool implementation
func (r *Tool) call{ToolName}Tool(ai models.AIInterface, params string) (response string, err error) {
    var jsonData = []byte(params)
    var pars {ToolName}ToolParams

    err = json.Unmarshal(jsonData, &pars)
    if err != nil {
        return fmt.Sprintf("Error parsing parameters: %v", err), err
    }

    // Validate required configuration
    if r.Endpoint == "" {
        return "Endpoint not configured", fmt.Errorf("missing endpoint")
    }

    // Implement switch based on action
    switch pars.Action {
    case "action1":
        return r.handleAction1(r.Endpoint, /* other params */)
    case "action2":
        return r.handleAction2(r.Endpoint, /* other params */)
    default:
        return fmt.Sprintf("Unknown action: %s", pars.Action), fmt.Errorf("unknown action")
    }
}

// Implement individual action handlers
func (r *Tool) handleAction1(baseURL string, /* params */) (string, error) {
    // Implementation here
}
```

#### 1.2 Create Comprehensive Tests
**File**: `backend/tool/{toolname}_test.go`

Key test patterns to include:
- Parameter validation tests
- HTTP client interaction tests (with httptest.Server)
- Error handling tests
- JSON schema validation tests

**Critical**: Implement a proper mock for `AIInterface`:
```go
type mockAIInterface struct{}

func (m *mockAIInterface) SendToConsole(event models.ConsoleEvent) {}
func (m *mockAIInterface) GetUser() models.User {
    return models.User{Username: "test", Fullname: "Test User", ADGroups: make(map[string]struct{})}
}
func (m *mockAIInterface) Completion(question string) (response string, consumption *models.TokenConsumption, err error) {
    return "mock response", nil, nil
}
func (m *mockAIInterface) GetContextValue(key string) (models.ContextField, bool) {
    return models.ContextField{}, false
}
```

**Test early and often**: Run tests after creating the implementation:
```bash
go test ./backend/tool -run Test{ToolName} -v
```

#### 1.3 Add Tool to Dispatcher
**File**: `backend/tool/calls.go`

Add case to the switch statement before the `default` case:
```go
case "{toolname}":
    resp, err = toolToCall.call{ToolName}Tool(ai, params)
    toolCall.Results = resp
    toolCall.Duration = time.Since(toolCall.StartedAt)
    toolCall.RAWResults = resp
    toolCall.RAWResultsCount = 1
    sendToolCallResultToConsole(ai, toolCall.ToolName, err, resp)
    return toolCall, false, err
```

#### 1.4 Register Tool Parameters
**File**: `backend/tool/toolDefinitions.go`

Add case to `OpenAIToolParams` function:
```go
case "{toolname}":
    p := {ToolName}ToolParamsJsonSchema
    return &p, true
```

#### 1.5 Verify Backend Build
```bash
go build ./backend/tool
go test ./backend/tool
```

### Phase 2: Frontend Implementation

#### 2.1 Add UI Template
**File**: `frontend/config/tools/tools.templ`

1. Add case to the switch statement in `ToolFields`:
```go
case "{toolname}":
    @{ToolName}Tool(t, errors)
```

2. Add the tool template at the end of the file:
```go
templ {ToolName}Tool(t ToolFormData, errors map[string]string) {
    <fieldset id="tool-definition">
        <legend>{Tool Name} Tool</legend>
        @ui.Field("Endpoint", t.Endpoint, "Enter {Tool Name} API URL", errors, true)
        @ui.Dropdown(t.TokenID, false, t.GetFieldOptions("TokenID"), "TokenID", "Choose authentication token (optional)", errors)
        @ui.Area("ParametersText", t.ParametersText, "Parameters (auto-generated)", errors, false, 12, "")
        <p><small>Brief description of tool capabilities</small></p>
    </fieldset>
}
```

#### 2.2 Add Handler Logic
**File**: `frontend/config/tools/tools.go`

1. **ToolDefinitionHandler**: Add case for new tool initialization:
```go
case "{toolname}":
    // Set default parameters
    params := tool.{ToolName}ToolParamsJsonSchema
    pb, _ := json.MarshalIndent(params, "", "  ")
    t.ParametersText = string(pb)
    component = {ToolName}Tool(t, nil)
```

2. **saveTool function**: Add validation logic:
```go
case "{toolname}":
    if new.Endpoint == "" {
        errors["Endpoint"] = "Endpoint URL is required"
    }
    // Validate endpoint URL if needed
    if err := validateHTTPURL(new.Endpoint); err != nil {
        errors["Endpoint"] = err.Error()
    }
    // Set parameters
    params := tool.{ToolName}ToolParamsJsonSchema
    new.Parameters = &params
    text, err := json.MarshalIndent(&params, "", "  ")
    if err != nil {
        errors["Parameters"] = "Invalid JSON: " + err.Error()
    }
    new.ParametersText = string(text)
```

3. **Endpoint handling**: If your tool needs endpoint/token, add to switch:
```go
// Endpoint
switch new.ToolType {
case "rest", "alertmanager", "{toolname}":
    // Keep the endpoint and token
default:
    new.Endpoint = ""
    new.TokenID = ""
}
```

#### 2.3 Generate Templates and Test
```bash
# CRITICAL: Generate templ files BEFORE building
templ generate ./frontend/config/tools

# Test compilation
go build ./frontend/config/tools

# Test full build
go build
```

**⚠️ Important**: Always run `templ generate` before `go build`. The Go compiler needs the generated `.go` files from the `.templ` templates. Building without generating templates first will result in "undefined" errors.

### Phase 3: Configuration and Integration

#### 3.1 Update Reference Configuration
**File**: `reference.yaml`

The tool type and description should already be added as mentioned in the initial request. Verify:
```yaml
tool_types:
  {toolname}: "{Tool Display Name}"

tool_type_descriptions:
  {toolname}: "Description of what this tool does"
```

#### 3.2 Final Integration Testing
```bash
# Run all tests
go test ./...

# Build entire project
go build

# Test specific tool
go test ./backend/tool -run Test{ToolName} -v
```

## Common Patterns and Best Practices

### Parameter Design
- **Always require an `action` parameter** for multi-function tools
- **Use descriptive enum values** for actions
- **Make optional parameters truly optional** with `omitempty` tags
- **Provide clear descriptions** for each parameter

### Error Handling
- **Validate inputs early** and return descriptive errors
- **Check required configuration** (endpoints, tokens) before processing
- **Use appropriate HTTP timeouts** for external calls
- **Return user-friendly error messages**

### Testing Strategy
- **Test parameter validation** thoroughly
- **Mock external services** using `httptest.Server`
- **Test both success and failure paths**
- **Verify JSON schema is valid**

### UI Considerations
- **Use consistent field patterns** (Endpoint, TokenID, ParametersText)
- **Provide helpful placeholder text** and descriptions
- **Show auto-generated parameters** as read-only
- **Include validation for required fields**

## Troubleshooting Common Issues

### Build Failures
1. **"undefined: {ToolName}Tool"**: Run `templ generate ./frontend/config/tools` before building
2. **Template-related build errors**: Always generate templates BEFORE building (`templ generate` → `go build`)
3. **Import cycle errors**: Check import statements in tool files
4. **Missing AIInterface methods**: Update mock with all required methods

### Test Failures
1. **Mock interface errors**: Ensure mock implements all `AIInterface` methods
2. **HTTP test issues**: Use `httptest.Server` for HTTP mocking
3. **JSON schema errors**: Validate schema structure manually

### Runtime Issues
1. **Tool not appearing**: Check `reference.yaml` configuration
2. **Parameters not working**: Verify `OpenAIToolParams` registration
3. **Endpoint errors**: Ensure URL validation is working

## Checklist for New Tool Implementation

### Backend
- [ ] Created `backend/tool/{toolname}.go` with tool implementation
- [ ] Added comprehensive tests in `{toolname}_test.go`
- [ ] Added tool case to `backend/tool/calls.go`
- [ ] Registered parameters in `backend/tool/toolDefinitions.go`
- [ ] Verified backend builds and tests pass

### Frontend  
- [ ] Added `{ToolName}Tool` template to `tools.templ`
- [ ] Added case to tool type switch in template
- [ ] Added initialization logic to `ToolDefinitionHandler`
- [ ] Added validation logic to `saveTool`
- [ ] Updated endpoint handling if needed
- [ ] **CRITICAL**: Generated templates with `templ generate ./frontend/config/tools`
- [ ] Verified frontend builds successfully (after template generation)

### Integration
- [ ] Confirmed `reference.yaml` has tool type and description
- [ ] Full project builds without errors
- [ ] All tests pass
- [ ] Tool appears in admin UI (manual verification)

## Time Estimate
- **Backend Implementation**: 2-3 hours
- **Frontend Implementation**: 1-2 hours  
- **Testing and Integration**: 1 hour
- **Total**: 4-6 hours for a standard tool

## Success Criteria
✅ Clean build with no errors  
✅ All tests passing  
✅ Tool appears in admin UI  
✅ Can create and configure tool instance  
✅ Tool responds to natural language queries  

---

*Remember: Test early, test often, and keep the build green throughout the process!*