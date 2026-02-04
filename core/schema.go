// Package core 提供 Hexagon 框架的核心接口和类型
//
// 本文件实现智能 Schema 生成功能：
//   - 反射生成：从 Go 类型自动生成 JSON Schema
//   - 注解支持：使用 struct tag 定义 Schema 属性
//   - 验证器：基于 Schema 的输入验证
//   - Schema 注册：全局 Schema 注册表
//
// 设计借鉴：
//   - JSON Schema: 标准规范
//   - Pydantic: Python 类型验证
//   - Zod: TypeScript Schema 验证
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ============== 错误定义 ==============

var (
	// ErrSchemaValidation Schema 验证错误
	ErrSchemaValidation = errors.New("schema validation failed")

	// ErrInvalidType 无效类型
	ErrInvalidType = errors.New("invalid type")

	// ErrRequiredField 缺少必需字段
	ErrRequiredField = errors.New("required field missing")

	// ErrPatternMismatch 模式不匹配
	ErrPatternMismatch = errors.New("pattern mismatch")
)

// ============== Schema 增强 ==============

// SchemaBuilder Schema 构建器
type SchemaBuilder struct {
	schema *Schema
}

// NewSchemaBuilder 创建 Schema 构建器
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		schema: &Schema{
			Properties: make(map[string]*Schema),
		},
	}
}

// Type 设置类型
func (b *SchemaBuilder) Type(t string) *SchemaBuilder {
	b.schema.Type = t
	return b
}

// Title 设置标题
func (b *SchemaBuilder) Title(title string) *SchemaBuilder {
	b.schema.Title = title
	return b
}

// Description 设置描述
func (b *SchemaBuilder) Description(desc string) *SchemaBuilder {
	b.schema.Description = desc
	return b
}

// Required 设置必需字段
func (b *SchemaBuilder) Required(fields ...string) *SchemaBuilder {
	b.schema.Required = append(b.schema.Required, fields...)
	return b
}

// Property 添加属性
func (b *SchemaBuilder) Property(name string, schema *Schema) *SchemaBuilder {
	b.schema.Properties[name] = schema
	return b
}

// Items 设置数组元素类型
func (b *SchemaBuilder) Items(schema *Schema) *SchemaBuilder {
	b.schema.Items = schema
	return b
}

// Enum 设置枚举值
func (b *SchemaBuilder) Enum(values ...any) *SchemaBuilder {
	b.schema.Enum = values
	return b
}

// Default 设置默认值
func (b *SchemaBuilder) Default(value any) *SchemaBuilder {
	b.schema.Default = value
	return b
}

// Pattern 设置正则模式
func (b *SchemaBuilder) Pattern(pattern string) *SchemaBuilder {
	b.schema.Pattern = pattern
	return b
}

// MinLength 设置最小长度
func (b *SchemaBuilder) MinLength(min int) *SchemaBuilder {
	b.schema.MinLength = &min
	return b
}

// MaxLength 设置最大长度
func (b *SchemaBuilder) MaxLength(max int) *SchemaBuilder {
	b.schema.MaxLength = &max
	return b
}

// Minimum 设置最小值
func (b *SchemaBuilder) Minimum(min float64) *SchemaBuilder {
	b.schema.Minimum = &min
	return b
}

// Maximum 设置最大值
func (b *SchemaBuilder) Maximum(max float64) *SchemaBuilder {
	b.schema.Maximum = &max
	return b
}

// Build 构建 Schema
func (b *SchemaBuilder) Build() *Schema {
	return b.schema
}

// ============== 智能 Schema 生成 ==============

// SchemaGenerator Schema 生成器
type SchemaGenerator struct {
	// DisallowUnknownFields 禁止未知字段
	DisallowUnknownFields bool

	// UseJSONTags 使用 json tag 作为属性名
	UseJSONTags bool

	// IncludePrivate 包含私有字段
	IncludePrivate bool

	// cache 缓存
	cache sync.Map
}

// NewSchemaGenerator 创建 Schema 生成器
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		UseJSONTags: true,
	}
}

// GenerateSchema 从 Go 类型生成 Schema
//
// 支持的 struct tags:
//   - json: JSON 属性名
//   - schema: Schema 属性 (type, format, description, required, default, enum, pattern, min, max)
//   - validate: 验证规则 (required, min, max, pattern, enum)
//
// 示例:
//
//	type User struct {
//	    Name  string `json:"name" schema:"description=用户名,required"`
//	    Age   int    `json:"age" schema:"min=0,max=150"`
//	    Email string `json:"email" schema:"format=email,pattern=.+@.+"`
//	}
//	schema := generator.GenerateSchema(User{})
func (g *SchemaGenerator) GenerateSchema(v any) *Schema {
	t := reflect.TypeOf(v)
	if t == nil {
		return &Schema{Type: "null"}
	}

	return g.generateForType(t)
}

// GenerateSchemaFromType 从类型生成 Schema
func (g *SchemaGenerator) GenerateSchemaFromType(t reflect.Type) *Schema {
	return g.generateForType(t)
}

func (g *SchemaGenerator) generateForType(t reflect.Type) *Schema {
	// 检查缓存
	if cached, ok := g.cache.Load(t); ok {
		return cached.(*Schema)
	}

	schema := g.doGenerateForType(t)

	// 存入缓存
	g.cache.Store(t, schema)
	return schema
}

func (g *SchemaGenerator) doGenerateForType(t reflect.Type) *Schema {
	// 处理指针
	if t.Kind() == reflect.Pointer {
		return g.generateForType(t.Elem())
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}

	case reflect.Bool:
		return &Schema{Type: "boolean"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{Type: "integer"}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer", Minimum: ptrFloat64(0)}

	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}

	case reflect.Slice, reflect.Array:
		return &Schema{
			Type:  "array",
			Items: g.generateForType(t.Elem()),
		}

	case reflect.Map:
		return &Schema{
			Type: "object",
			AdditionalProperties: g.generateForType(t.Elem()),
		}

	case reflect.Struct:
		return g.generateStructSchema(t)

	case reflect.Interface:
		return &Schema{} // any

	default:
		return &Schema{Type: "string"}
	}
}

func (g *SchemaGenerator) generateStructSchema(t reflect.Type) *Schema {
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过私有字段
		if !g.IncludePrivate && field.PkgPath != "" {
			continue
		}

		// 获取属性名
		name := field.Name
		if g.UseJSONTags {
			if tag := field.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					name = parts[0]
				}
			}
		}

		// 生成字段 Schema
		propSchema := g.generateForType(field.Type)

		// 解析 schema tag
		if tag := field.Tag.Get("schema"); tag != "" {
			g.parseSchemaTag(propSchema, tag)
			if strings.Contains(tag, "required") {
				schema.Required = append(schema.Required, name)
			}
		}

		// 解析 validate tag
		if tag := field.Tag.Get("validate"); tag != "" {
			g.parseValidateTag(propSchema, tag, &schema.Required, name)
		}

		// 解析 desc/description tag
		if desc := field.Tag.Get("desc"); desc != "" {
			propSchema.Description = desc
		} else if desc := field.Tag.Get("description"); desc != "" {
			propSchema.Description = desc
		}

		schema.Properties[name] = propSchema
	}

	return schema
}

func (g *SchemaGenerator) parseSchemaTag(schema *Schema, tag string) {
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		key := strings.TrimSpace(kv[0])
		value := ""
		if len(kv) > 1 {
			value = strings.TrimSpace(kv[1])
		}

		switch key {
		case "type":
			schema.Type = value
		case "format":
			schema.Format = value
		case "description", "desc":
			schema.Description = value
		case "default":
			schema.Default = parseValue(value, schema.Type)
		case "enum":
			schema.Enum = parseEnumValues(value)
		case "pattern":
			schema.Pattern = value
		case "min":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Minimum = &f
			}
		case "max":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Maximum = &f
			}
		case "minLength":
			if i, err := strconv.Atoi(value); err == nil {
				schema.MinLength = &i
			}
		case "maxLength":
			if i, err := strconv.Atoi(value); err == nil {
				schema.MaxLength = &i
			}
		}
	}
}

func (g *SchemaGenerator) parseValidateTag(schema *Schema, tag string, required *[]string, name string) {
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		key := strings.TrimSpace(kv[0])
		value := ""
		if len(kv) > 1 {
			value = strings.TrimSpace(kv[1])
		}

		switch key {
		case "required":
			*required = append(*required, name)
		case "min":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Minimum = &f
			}
		case "max":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Maximum = &f
			}
		case "pattern":
			schema.Pattern = value
		case "enum":
			schema.Enum = parseEnumValues(value)
		}
	}
}

func parseValue(value, schemaType string) any {
	switch schemaType {
	case "integer":
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	case "number":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	case "boolean":
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return value
}

func parseEnumValues(value string) []any {
	parts := strings.Split(value, "|")
	values := make([]any, len(parts))
	for i, p := range parts {
		values[i] = strings.TrimSpace(p)
	}
	return values
}

func ptrFloat64(f float64) *float64 {
	return &f
}

// ============== Schema 验证 ==============

// Validator Schema 验证器
type Validator struct {
	generator *SchemaGenerator
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{
		generator: NewSchemaGenerator(),
	}
}

// Validate 验证数据是否符合 Schema
func (v *Validator) Validate(schema *Schema, data any) error {
	return v.validateValue(schema, reflect.ValueOf(data), "")
}

// ValidateJSON 验证 JSON 数据
func (v *Validator) ValidateJSON(schema *Schema, jsonData []byte) error {
	var data any
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", ErrSchemaValidation, err)
	}
	return v.Validate(schema, data)
}

func (v *Validator) validateValue(schema *Schema, val reflect.Value, path string) error {
	if !val.IsValid() {
		return nil
	}

	// 处理指针
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		return v.validateValue(schema, val.Elem(), path)
	}

	// 处理 interface
	if val.Kind() == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		return v.validateValue(schema, val.Elem(), path)
	}

	// 类型验证
	if schema.Type != "" {
		if err := v.validateType(schema.Type, val, path); err != nil {
			return err
		}
	}

	// 枚举验证
	if len(schema.Enum) > 0 {
		if err := v.validateEnum(schema.Enum, val, path); err != nil {
			return err
		}
	}

	// 字符串验证
	if schema.Type == "string" && val.Kind() == reflect.String {
		if err := v.validateString(schema, val.String(), path); err != nil {
			return err
		}
	}

	// 数字验证
	if (schema.Type == "number" || schema.Type == "integer") &&
		(val.Kind() >= reflect.Int && val.Kind() <= reflect.Float64) {
		var num float64
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			num = float64(val.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			num = float64(val.Uint())
		case reflect.Float32, reflect.Float64:
			num = val.Float()
		}
		if err := v.validateNumber(schema, num, path); err != nil {
			return err
		}
	}

	// 对象验证
	if schema.Type == "object" && val.Kind() == reflect.Map {
		if err := v.validateObject(schema, val, path); err != nil {
			return err
		}
	}

	// 数组验证
	if schema.Type == "array" && (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) {
		if err := v.validateArray(schema, val, path); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateType(schemaType string, val reflect.Value, path string) error {
	valid := false

	switch schemaType {
	case "string":
		valid = val.Kind() == reflect.String
	case "integer":
		valid = val.Kind() >= reflect.Int && val.Kind() <= reflect.Uint64
	case "number":
		valid = val.Kind() >= reflect.Int && val.Kind() <= reflect.Float64
	case "boolean":
		valid = val.Kind() == reflect.Bool
	case "array":
		valid = val.Kind() == reflect.Slice || val.Kind() == reflect.Array
	case "object":
		valid = val.Kind() == reflect.Map || val.Kind() == reflect.Struct
	case "null":
		valid = !val.IsValid() || (val.Kind() == reflect.Pointer && val.IsNil())
	default:
		valid = true
	}

	if !valid {
		return fmt.Errorf("%w: %s: expected %s, got %s", ErrInvalidType, path, schemaType, val.Kind())
	}
	return nil
}

func (v *Validator) validateEnum(enum []any, val reflect.Value, path string) error {
	valInterface := val.Interface()
	for _, e := range enum {
		if reflect.DeepEqual(valInterface, e) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s: value not in enum", ErrSchemaValidation, path)
}

func (v *Validator) validateString(schema *Schema, s string, path string) error {
	if schema.MinLength != nil && len(s) < *schema.MinLength {
		return fmt.Errorf("%w: %s: string too short (min: %d)", ErrSchemaValidation, path, *schema.MinLength)
	}
	if schema.MaxLength != nil && len(s) > *schema.MaxLength {
		return fmt.Errorf("%w: %s: string too long (max: %d)", ErrSchemaValidation, path, *schema.MaxLength)
	}
	if schema.Pattern != "" {
		re, err := regexp.Compile(schema.Pattern)
		if err != nil {
			return fmt.Errorf("%w: %s: invalid pattern: %v", ErrSchemaValidation, path, err)
		}
		if !re.MatchString(s) {
			return fmt.Errorf("%w: %s: pattern mismatch", ErrPatternMismatch, path)
		}
	}
	return nil
}

func (v *Validator) validateNumber(schema *Schema, n float64, path string) error {
	if schema.Minimum != nil && n < *schema.Minimum {
		return fmt.Errorf("%w: %s: number too small (min: %v)", ErrSchemaValidation, path, *schema.Minimum)
	}
	if schema.Maximum != nil && n > *schema.Maximum {
		return fmt.Errorf("%w: %s: number too large (max: %v)", ErrSchemaValidation, path, *schema.Maximum)
	}
	return nil
}

func (v *Validator) validateObject(schema *Schema, val reflect.Value, path string) error {
	// 验证必需字段
	for _, req := range schema.Required {
		found := false
		iter := val.MapRange()
		for iter.Next() {
			if iter.Key().String() == req {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: %s.%s", ErrRequiredField, path, req)
		}
	}

	// 验证每个属性
	iter := val.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		propSchema, ok := schema.Properties[key]
		if !ok {
			propSchema = schema.AdditionalProperties
		}
		if propSchema != nil {
			propPath := key
			if path != "" {
				propPath = path + "." + key
			}
			if err := v.validateValue(propSchema, iter.Value(), propPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (v *Validator) validateArray(schema *Schema, val reflect.Value, path string) error {
	if schema.Items == nil {
		return nil
	}

	for i := 0; i < val.Len(); i++ {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if err := v.validateValue(schema.Items, val.Index(i), itemPath); err != nil {
			return err
		}
	}

	return nil
}

// ============== Schema 注册表 ==============

// SchemaRegistry Schema 注册表
type SchemaRegistry struct {
	schemas   map[string]*Schema
	generator *SchemaGenerator
	mu        sync.RWMutex
}

// NewSchemaRegistry 创建 Schema 注册表
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas:   make(map[string]*Schema),
		generator: NewSchemaGenerator(),
	}
}

// Register 注册 Schema
func (r *SchemaRegistry) Register(name string, schema *Schema) {
	r.mu.Lock()
	r.schemas[name] = schema
	r.mu.Unlock()
}

// RegisterType 从类型注册 Schema
func (r *SchemaRegistry) RegisterType(name string, v any) {
	schema := r.generator.GenerateSchema(v)
	r.Register(name, schema)
}

// Get 获取 Schema
func (r *SchemaRegistry) Get(name string) (*Schema, bool) {
	r.mu.RLock()
	schema, ok := r.schemas[name]
	r.mu.RUnlock()
	return schema, ok
}

// List 列出所有 Schema
func (r *SchemaRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.schemas))
	for name := range r.schemas {
		names = append(names, name)
	}
	return names
}

// GlobalSchemaRegistry 全局 Schema 注册表
var GlobalSchemaRegistry = NewSchemaRegistry()

// ============== 快捷函数 ==============

// GenerateSchema 生成 Schema（使用默认生成器）
func GenerateSchema(v any) *Schema {
	return NewSchemaGenerator().GenerateSchema(v)
}

// ValidateWithSchema 验证数据是否符合 Schema
func ValidateWithSchema(schema *Schema, data any) error {
	return NewValidator().Validate(schema, data)
}
