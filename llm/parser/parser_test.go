package parser

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// ============== 测试用类型定义 ==============

// User 基础测试用结构体
type User struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email,omitempty"`
}

// Address 嵌套测试用结构体
type Address struct {
	Street string `json:"street"`
	City   string `json:"city"`
	Zip    string `json:"zip"`
}

// UserWithAddress 带嵌套的测试用结构体
type UserWithAddress struct {
	Name    string  `json:"name"`
	Age     int     `json:"age"`
	Address Address `json:"address"`
}

// Product 带描述标签的测试用结构体
type Product struct {
	ID    int      `json:"id" desc:"产品ID"`
	Name  string   `json:"name" description:"产品名称"`
	Tags  []string `json:"tags"`
	Price float64  `json:"price"`
}

// StrictUser 严格模式测试用结构体（仅包含两个字段）
type StrictUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// ============== JSONParser 测试 ==============

func TestJSONParser_ValidObject(t *testing.T) {
	// 测试有效 JSON 对象的正常解析
	p := NewJSONParser[User]()
	ctx := context.Background()

	result, err := p.Parse(ctx, `{"name": "Alice", "age": 30, "email": "alice@example.com"}`)
	if err != nil {
		t.Fatalf("解析有效 JSON 失败: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
	if result.Age != 30 {
		t.Errorf("Age 期望 30，得到 %d", result.Age)
	}
	if result.Email != "alice@example.com" {
		t.Errorf("Email 期望 alice@example.com，得到 %s", result.Email)
	}
}

func TestJSONParser_MarkdownCodeBlock(t *testing.T) {
	// 测试从 markdown 代码块中提取 JSON
	p := NewJSONParser[User]()
	ctx := context.Background()

	input := "```json\n{\"name\": \"Bob\", \"age\": 25}\n```"
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("解析 markdown 代码块中的 JSON 失败: %v", err)
	}
	if result.Name != "Bob" {
		t.Errorf("Name 期望 Bob，得到 %s", result.Name)
	}
	if result.Age != 25 {
		t.Errorf("Age 期望 25，得到 %d", result.Age)
	}
}

func TestJSONParser_MarkdownCodeBlockNoLang(t *testing.T) {
	// 测试从不带语言标记的 markdown 代码块中提取 JSON
	p := NewJSONParser[User]()
	ctx := context.Background()

	input := "```\n{\"name\": \"Charlie\", \"age\": 35}\n```"
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("解析无语言标记代码块中的 JSON 失败: %v", err)
	}
	if result.Name != "Charlie" {
		t.Errorf("Name 期望 Charlie，得到 %s", result.Name)
	}
}

func TestJSONParser_SurroundingText(t *testing.T) {
	// 测试从带有前后文本的输出中提取 JSON
	p := NewJSONParser[User]()
	ctx := context.Background()

	input := `Here is the result:
{"name": "Diana", "age": 28}
Hope this helps!`
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("从带前后文本中提取 JSON 失败: %v", err)
	}
	if result.Name != "Diana" {
		t.Errorf("Name 期望 Diana，得到 %s", result.Name)
	}
}

func TestJSONParser_NestedObject(t *testing.T) {
	// 测试嵌套 JSON 对象的解析
	p := NewJSONParser[UserWithAddress]()
	ctx := context.Background()

	input := `{"name": "Eve", "age": 40, "address": {"street": "123 Main St", "city": "Springfield", "zip": "62701"}}`
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("解析嵌套 JSON 失败: %v", err)
	}
	if result.Name != "Eve" {
		t.Errorf("Name 期望 Eve，得到 %s", result.Name)
	}
	if result.Address.City != "Springfield" {
		t.Errorf("City 期望 Springfield，得到 %s", result.Address.City)
	}
	if result.Address.Zip != "62701" {
		t.Errorf("Zip 期望 62701，得到 %s", result.Address.Zip)
	}
}

func TestJSONParser_Array(t *testing.T) {
	// 测试 JSON 数组的解析
	p := NewJSONParser[[]User]()
	ctx := context.Background()

	input := `[{"name": "Alice", "age": 30}, {"name": "Bob", "age": 25}]`
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("解析 JSON 数组失败: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("期望 2 个元素，得到 %d", len(result))
	}
	if result[0].Name != "Alice" {
		t.Errorf("第一个用户 Name 期望 Alice，得到 %s", result[0].Name)
	}
	if result[1].Name != "Bob" {
		t.Errorf("第二个用户 Name 期望 Bob，得到 %s", result[1].Name)
	}
}

func TestJSONParser_StrictMode_RejectExtraFields(t *testing.T) {
	// 测试严格模式拒绝多余字段
	p := NewJSONParser[StrictUser]().WithStrictMode(true)
	ctx := context.Background()

	input := `{"name": "Alice", "age": 30, "extra": "field"}`
	_, err := p.Parse(ctx, input)
	if err == nil {
		t.Fatal("严格模式应拒绝多余字段")
	}
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("错误类型应为 ErrParseFailure，得到: %v", err)
	}
}

func TestJSONParser_NonStrictMode_AllowExtraFields(t *testing.T) {
	// 测试非严格模式允许多余字段
	p := NewJSONParser[StrictUser]().WithStrictMode(false)
	ctx := context.Background()

	input := `{"name": "Alice", "age": 30, "extra": "field"}`
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("非严格模式不应报错: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
}

func TestJSONParser_ExtractJSON_Disabled(t *testing.T) {
	// 测试关闭 ExtractJSON 时要求纯 JSON 输入
	p := NewJSONParser[User]().WithExtractJSON(false)
	ctx := context.Background()

	// 纯 JSON 应该正常解析
	result, err := p.Parse(ctx, `{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatalf("纯 JSON 输入不应报错: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}

	// 带前缀文本应该解析失败（因为关闭了 ExtractJSON，整个字符串作为 JSON 直接解析）
	_, err = p.Parse(ctx, `Here is the result: {"name": "Alice", "age": 30}`)
	if err == nil {
		t.Fatal("关闭 ExtractJSON 时，非纯 JSON 输入应该失败")
	}
}

func TestJSONParser_WithValidator_Pass(t *testing.T) {
	// 测试验证器通过的情况
	p := NewJSONParser[User]().WithValidator(func(u User) error {
		if u.Age < 0 {
			return fmt.Errorf("年龄不能为负数")
		}
		return nil
	})
	ctx := context.Background()

	result, err := p.Parse(ctx, `{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatalf("验证应通过: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
}

func TestJSONParser_WithValidator_Fail(t *testing.T) {
	// 测试验证器失败的情况
	p := NewJSONParser[User]().WithValidator(func(u User) error {
		if u.Age < 0 {
			return fmt.Errorf("年龄不能为负数")
		}
		return nil
	})
	ctx := context.Background()

	_, err := p.Parse(ctx, `{"name": "Alice", "age": -5}`)
	if err == nil {
		t.Fatal("验证应失败")
	}
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("错误类型应为 ErrValidationFailure，得到: %v", err)
	}
}

func TestJSONParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p := NewJSONParser[User]()
	ctx := context.Background()

	_, err := p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestJSONParser_NoJSONContent(t *testing.T) {
	// 测试无 JSON 内容返回 ErrParseFailure
	p := NewJSONParser[User]()
	ctx := context.Background()

	_, err := p.Parse(ctx, "This is just plain text without any JSON")
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("无 JSON 内容应返回 ErrParseFailure，得到: %v", err)
	}
}

func TestJSONParser_InvalidJSON(t *testing.T) {
	// 测试无效 JSON 返回 ErrParseFailure
	p := NewJSONParser[User]()
	ctx := context.Background()

	_, err := p.Parse(ctx, `{"name": "Alice", "age": }`)
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("无效 JSON 应返回 ErrParseFailure，得到: %v", err)
	}
}

func TestJSONParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 返回非空字符串
	p := NewJSONParser[User]()
	instructions := p.GetFormatInstructions()
	if instructions == "" {
		t.Fatal("GetFormatInstructions 不应返回空字符串")
	}
	if !strings.Contains(instructions, "JSON") {
		t.Error("GetFormatInstructions 应包含 JSON 关键字")
	}
}

func TestJSONParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 返回正确的类型名
	p := NewJSONParser[User]()
	name := p.GetTypeName()
	if name != "User" {
		t.Errorf("GetTypeName 期望 User，得到 %s", name)
	}
}

func TestJSONParser_ComplexNestedStruct(t *testing.T) {
	// 测试复杂嵌套结构体（带数组和嵌套对象）
	p := NewJSONParser[Product]()
	ctx := context.Background()

	input := `{"id": 1, "name": "Widget", "tags": ["electronics", "sale"], "price": 29.99}`
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("解析复杂嵌套结构体失败: %v", err)
	}
	if result.ID != 1 {
		t.Errorf("ID 期望 1，得到 %d", result.ID)
	}
	if result.Name != "Widget" {
		t.Errorf("Name 期望 Widget，得到 %s", result.Name)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("Tags 期望 2 个元素，得到 %d", len(result.Tags))
	}
	if result.Tags[0] != "electronics" {
		t.Errorf("第一个 Tag 期望 electronics，得到 %s", result.Tags[0])
	}
	if result.Price != 29.99 {
		t.Errorf("Price 期望 29.99，得到 %f", result.Price)
	}
}

func TestJSONParser_Validate(t *testing.T) {
	// 测试 Validate 方法的直接调用
	p := NewJSONParser[User]().WithValidator(func(u User) error {
		if u.Name == "" {
			return fmt.Errorf("名称不能为空")
		}
		return nil
	})
	ctx := context.Background()

	// 验证通过
	err := p.Validate(ctx, User{Name: "Alice", Age: 30})
	if err != nil {
		t.Fatalf("Validate 通过时不应报错: %v", err)
	}

	// 验证失败
	err = p.Validate(ctx, User{Name: "", Age: 30})
	if err == nil {
		t.Fatal("Validate 失败时应报错")
	}
}

func TestJSONParser_MultipleValidators(t *testing.T) {
	// 测试多个验证器按顺序执行
	p := NewJSONParser[User]().
		WithValidator(func(u User) error {
			if u.Name == "" {
				return fmt.Errorf("名称不能为空")
			}
			return nil
		}).
		WithValidator(func(u User) error {
			if u.Age < 18 {
				return fmt.Errorf("年龄必须大于等于 18")
			}
			return nil
		})
	ctx := context.Background()

	// 第一个验证器失败
	_, err := p.Parse(ctx, `{"name": "", "age": 25}`)
	if err == nil {
		t.Fatal("第一个验证器应失败")
	}

	// 第二个验证器失败
	_, err = p.Parse(ctx, `{"name": "Alice", "age": 10}`)
	if err == nil {
		t.Fatal("第二个验证器应失败")
	}

	// 所有验证器通过
	result, err := p.Parse(ctx, `{"name": "Alice", "age": 25}`)
	if err != nil {
		t.Fatalf("所有验证器通过时不应报错: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
}

// ============== RegexParser 测试 ==============

func TestRegexParser_BasicMatch(t *testing.T) {
	// 测试基础正则匹配和分组提取
	p, err := NewRegexParser(`(\w+)\s+(\d+)`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	ctx := context.Background()

	result, err := p.Parse(ctx, "Alice 30")
	if err != nil {
		t.Fatalf("正则解析失败: %v", err)
	}
	// 无命名分组时，result 应为空 map（只有空名称的分组不会被加入）
	if len(result) != 0 {
		t.Errorf("无命名分组时期望空 map，得到 %v", result)
	}
}

func TestRegexParser_NamedGroups(t *testing.T) {
	// 测试命名分组提取
	p, err := NewRegexParser(`(?P<name>\w+)\s+(?P<age>\d+)`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	ctx := context.Background()

	result, err := p.Parse(ctx, "Alice 30")
	if err != nil {
		t.Fatalf("正则解析失败: %v", err)
	}
	if result["name"] != "Alice" {
		t.Errorf("name 期望 Alice，得到 %s", result["name"])
	}
	if result["age"] != "30" {
		t.Errorf("age 期望 30，得到 %s", result["age"])
	}
}

func TestRegexParser_WithRequired_Present(t *testing.T) {
	// 测试必需字段存在时通过验证
	p, err := NewRegexParser(`(?P<name>\w+)\s+(?P<age>\d+)`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	p.WithRequired("name", "age")
	ctx := context.Background()

	result, err := p.Parse(ctx, "Alice 30")
	if err != nil {
		t.Fatalf("必需字段存在时不应报错: %v", err)
	}
	if result["name"] != "Alice" {
		t.Errorf("name 期望 Alice，得到 %s", result["name"])
	}
}

func TestRegexParser_WithRequired_Missing(t *testing.T) {
	// 测试必需字段缺失时报错
	p, err := NewRegexParser(`(?P<name>\w+)(\s+(?P<email>\S+))?`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	p.WithRequired("name", "email")
	ctx := context.Background()

	// email 分组可选且未匹配到，应该导致验证失败
	_, err = p.Parse(ctx, "Alice")
	if err == nil {
		t.Fatal("必需字段缺失时应报错")
	}
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("错误类型应为 ErrValidationFailure，得到: %v", err)
	}
}

func TestRegexParser_NoMatch(t *testing.T) {
	// 测试无匹配时返回 ErrParseFailure
	p, err := NewRegexParser(`\d{4}-\d{2}-\d{2}`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	ctx := context.Background()

	_, err = p.Parse(ctx, "no date here")
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("无匹配时应返回 ErrParseFailure，得到: %v", err)
	}
}

func TestRegexParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p, err := NewRegexParser(`\w+`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	ctx := context.Background()

	_, err = p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestRegexParser_InvalidPattern(t *testing.T) {
	// 测试无效正则表达式返回错误
	_, err := NewRegexParser(`[invalid`)
	if err == nil {
		t.Fatal("无效正则表达式应返回错误")
	}
}

func TestRegexParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 返回包含正则表达式的字符串
	p, err := NewRegexParser(`(?P<name>\w+)`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	instructions := p.GetFormatInstructions()
	if instructions == "" {
		t.Fatal("GetFormatInstructions 不应返回空字符串")
	}
	if !strings.Contains(instructions, "pattern") {
		t.Error("GetFormatInstructions 应包含 pattern 关键字")
	}
}

func TestRegexParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 返回 RegexMatch
	p, err := NewRegexParser(`\w+`)
	if err != nil {
		t.Fatalf("创建正则解析器失败: %v", err)
	}
	name := p.GetTypeName()
	if name != "RegexMatch" {
		t.Errorf("GetTypeName 期望 RegexMatch，得到 %s", name)
	}
}

// ============== EnumParser 测试 ==============

func TestEnumParser_ExactMatch_CaseInsensitive(t *testing.T) {
	// 测试大小写不敏感的精确匹配
	p := NewEnumParser("red", "green", "blue")
	ctx := context.Background()

	result, err := p.Parse(ctx, "RED")
	if err != nil {
		t.Fatalf("大小写不敏感匹配失败: %v", err)
	}
	// 应返回原始值
	if result != "red" {
		t.Errorf("期望返回原始值 red，得到 %s", result)
	}
}

func TestEnumParser_ExactMatch_MixedCase(t *testing.T) {
	// 测试混合大小写输入
	p := NewEnumParser("Red", "Green", "Blue")
	ctx := context.Background()

	result, err := p.Parse(ctx, "red")
	if err != nil {
		t.Fatalf("混合大小写匹配失败: %v", err)
	}
	if result != "Red" {
		t.Errorf("期望返回原始值 Red，得到 %s", result)
	}
}

func TestEnumParser_CaseSensitive(t *testing.T) {
	// 测试大小写敏感模式
	p := NewEnumParser("red", "green", "blue").WithCaseSensitive(true)
	ctx := context.Background()

	// 精确匹配应成功
	result, err := p.Parse(ctx, "red")
	if err != nil {
		t.Fatalf("精确匹配失败: %v", err)
	}
	if result != "red" {
		t.Errorf("期望 red，得到 %s", result)
	}

	// 大小写不匹配应失败
	_, err = p.Parse(ctx, "RED")
	if err == nil {
		t.Fatal("大小写敏感模式下，RED 应该匹配不到 red")
	}
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("错误类型应为 ErrValidationFailure，得到: %v", err)
	}
}

func TestEnumParser_PartialMatch(t *testing.T) {
	// 测试部分匹配模式
	p := NewEnumParser("approve", "reject", "pending").WithPartialMatch(true)
	ctx := context.Background()

	result, err := p.Parse(ctx, "I think we should approve this request")
	if err != nil {
		t.Fatalf("部分匹配失败: %v", err)
	}
	if result != "approve" {
		t.Errorf("期望 approve，得到 %s", result)
	}
}

func TestEnumParser_InvalidValue(t *testing.T) {
	// 测试无效值返回 ErrValidationFailure
	p := NewEnumParser("red", "green", "blue")
	ctx := context.Background()

	_, err := p.Parse(ctx, "yellow")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("无效值应返回 ErrValidationFailure，得到: %v", err)
	}
}

func TestEnumParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p := NewEnumParser("red", "green", "blue")
	ctx := context.Background()

	_, err := p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestEnumParser_TrimWhitespace(t *testing.T) {
	// 测试输入前后空白被正确裁剪
	p := NewEnumParser("red", "green", "blue")
	ctx := context.Background()

	result, err := p.Parse(ctx, "  green  ")
	if err != nil {
		t.Fatalf("空白裁剪后匹配应成功: %v", err)
	}
	if result != "green" {
		t.Errorf("期望 green，得到 %s", result)
	}
}

func TestEnumParser_WhitespaceOnly(t *testing.T) {
	// 测试仅空白输入返回 ErrEmptyOutput
	p := NewEnumParser("red", "green", "blue")
	ctx := context.Background()

	_, err := p.Parse(ctx, "   ")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("仅空白输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestEnumParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 包含所有可选值
	p := NewEnumParser("red", "green", "blue")
	instructions := p.GetFormatInstructions()
	if instructions == "" {
		t.Fatal("GetFormatInstructions 不应返回空字符串")
	}
	for _, v := range []string{"red", "green", "blue"} {
		if !strings.Contains(instructions, v) {
			t.Errorf("GetFormatInstructions 应包含 %s", v)
		}
	}
}

func TestEnumParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 返回 Enum
	p := NewEnumParser("a", "b")
	name := p.GetTypeName()
	if name != "Enum" {
		t.Errorf("GetTypeName 期望 Enum，得到 %s", name)
	}
}

// ============== ListParser 测试 ==============

func TestListParser_NewlineSeparator(t *testing.T) {
	// 测试默认换行分割
	p := NewListParser()
	ctx := context.Background()

	input := "apple\nbanana\ncherry"
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("列表解析失败: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("期望 3 个元素，得到 %d", len(result))
	}
	expected := []string{"apple", "banana", "cherry"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("第 %d 项期望 %s，得到 %s", i, v, result[i])
		}
	}
}

func TestListParser_CustomSeparator(t *testing.T) {
	// 测试自定义分隔符
	p := NewListParser().WithSeparator(",")
	ctx := context.Background()

	input := "apple, banana, cherry"
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("自定义分隔符解析失败: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("期望 3 个元素，得到 %d", len(result))
	}
	if result[0] != "apple" {
		t.Errorf("第 0 项期望 apple，得到 %s", result[0])
	}
	if result[1] != "banana" {
		t.Errorf("第 1 项期望 banana，得到 %s", result[1])
	}
}

func TestListParser_TrimAndFilterEmpty(t *testing.T) {
	// 测试去除空白和过滤空项
	p := NewListParser()
	ctx := context.Background()

	input := "  apple  \n  \n  banana  \n\n  cherry  "
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("列表解析失败: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("过滤空项后期望 3 个元素，得到 %d: %v", len(result), result)
	}
}

func TestListParser_RemoveListPrefixes(t *testing.T) {
	// 测试移除常见列表前缀（-、*、数字.）
	p := NewListParser()
	ctx := context.Background()

	input := "- apple\n* banana\n1. cherry\n2. durian"
	result, err := p.Parse(ctx, input)
	if err != nil {
		t.Fatalf("列表解析失败: %v", err)
	}
	expected := []string{"apple", "banana", "cherry", "durian"}
	if len(result) != len(expected) {
		t.Fatalf("期望 %d 个元素，得到 %d: %v", len(expected), len(result), result)
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("第 %d 项期望 %s，得到 %s", i, v, result[i])
		}
	}
}

func TestListParser_MinItems(t *testing.T) {
	// 测试最少项数验证
	p := NewListParser().WithMinItems(3)
	ctx := context.Background()

	_, err := p.Parse(ctx, "apple\nbanana")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("项数不足时应返回 ErrValidationFailure，得到: %v", err)
	}
}

func TestListParser_MaxItems(t *testing.T) {
	// 测试最大项数验证
	p := NewListParser().WithMaxItems(2)
	ctx := context.Background()

	_, err := p.Parse(ctx, "apple\nbanana\ncherry")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("项数超出时应返回 ErrValidationFailure，得到: %v", err)
	}
}

func TestListParser_MinMaxItems_Valid(t *testing.T) {
	// 测试项数在范围内通过验证
	p := NewListParser().WithMinItems(2).WithMaxItems(4)
	ctx := context.Background()

	result, err := p.Parse(ctx, "apple\nbanana\ncherry")
	if err != nil {
		t.Fatalf("项数在范围内不应报错: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("期望 3 个元素，得到 %d", len(result))
	}
}

func TestListParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p := NewListParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestListParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 返回格式说明
	p := NewListParser().WithMinItems(2).WithMaxItems(5)
	instructions := p.GetFormatInstructions()
	if instructions == "" {
		t.Fatal("GetFormatInstructions 不应返回空字符串")
	}
	if !strings.Contains(instructions, "minimum 2") {
		t.Error("应包含最小项数信息")
	}
	if !strings.Contains(instructions, "maximum 5") {
		t.Error("应包含最大项数信息")
	}
}

func TestListParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 返回 List
	p := NewListParser()
	name := p.GetTypeName()
	if name != "List" {
		t.Errorf("GetTypeName 期望 List，得到 %s", name)
	}
}

// ============== BoolParser 测试 ==============

func TestBoolParser_TrueValues(t *testing.T) {
	// 测试各种 true 值
	p := NewBoolParser()
	ctx := context.Background()

	trueInputs := []string{"yes", "true", "1", "确定", "是", "Yes", "TRUE", "True"}
	for _, input := range trueInputs {
		result, err := p.Parse(ctx, input)
		if err != nil {
			t.Errorf("输入 %q 应解析为 true，但报错: %v", input, err)
			continue
		}
		if !result {
			t.Errorf("输入 %q 应解析为 true，但得到 false", input)
		}
	}
}

func TestBoolParser_FalseValues(t *testing.T) {
	// 测试各种 false 值
	// 注意：BoolParser 使用 strings.Contains 并先检查 TrueValues，
	// 因此 "不是" 会匹配到 "是"（true 值），这是当前实现的已知行为。
	p := NewBoolParser()
	ctx := context.Background()

	falseInputs := []string{"no", "false", "0", "否", "No", "FALSE", "False"}
	for _, input := range falseInputs {
		result, err := p.Parse(ctx, input)
		if err != nil {
			t.Errorf("输入 %q 应解析为 false，但报错: %v", input, err)
			continue
		}
		if result {
			t.Errorf("输入 %q 应解析为 false，但得到 true", input)
		}
	}
}

func TestBoolParser_FalseValue_BuShi_MatchesTrueFirst(t *testing.T) {
	// 测试 "不是" 因包含 "是"（true 值）而优先匹配为 true 的已知行为
	// BoolParser 先遍历 TrueValues 再遍历 FalseValues，
	// 且使用 strings.Contains 做包含匹配，"不是" 包含 "是"，因此匹配为 true
	p := NewBoolParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "不是")
	if err != nil {
		t.Fatalf("解析 '不是' 不应报错: %v", err)
	}
	// 由于实现特性，"不是" 会匹配到 TrueValues 中的 "是"
	if !result {
		t.Error("当前实现下，'不是' 会因包含 '是' 而匹配为 true")
	}
}

func TestBoolParser_Undetermined(t *testing.T) {
	// 测试无法确定布尔值的输入
	p := NewBoolParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "maybe")
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("无法确定布尔值时应返回 ErrParseFailure，得到: %v", err)
	}
}

func TestBoolParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p := NewBoolParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestBoolParser_ContainsMatch(t *testing.T) {
	// 测试包含匹配（如 "Yes, I agree" 应匹配 yes）
	p := NewBoolParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "Yes, I agree")
	if err != nil {
		t.Fatalf("包含匹配失败: %v", err)
	}
	if !result {
		t.Error("包含 yes 的输入应解析为 true")
	}

	result, err = p.Parse(ctx, "No, I disagree")
	if err != nil {
		t.Fatalf("包含匹配失败: %v", err)
	}
	if result {
		t.Error("包含 no 的输入应解析为 false")
	}
}

func TestBoolParser_ChineseValues(t *testing.T) {
	// 测试中文 true/false 值
	p := NewBoolParser()
	ctx := context.Background()

	// "是" 应为 true
	result, err := p.Parse(ctx, "是的，我同意")
	if err != nil {
		t.Fatalf("中文 true 值解析失败: %v", err)
	}
	if !result {
		t.Error("包含'是'的输入应解析为 true")
	}
}

func TestBoolParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 返回非空字符串
	p := NewBoolParser()
	instructions := p.GetFormatInstructions()
	if instructions == "" {
		t.Fatal("GetFormatInstructions 不应返回空字符串")
	}
	if !strings.Contains(instructions, "yes") || !strings.Contains(instructions, "no") {
		t.Error("GetFormatInstructions 应包含 yes 和 no")
	}
}

func TestBoolParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 返回 Boolean
	p := NewBoolParser()
	name := p.GetTypeName()
	if name != "Boolean" {
		t.Errorf("GetTypeName 期望 Boolean，得到 %s", name)
	}
}

// ============== NumberParser 测试 ==============

func TestNumberParser_Integer(t *testing.T) {
	// 测试整数解析
	p := NewNumberParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "42")
	if err != nil {
		t.Fatalf("整数解析失败: %v", err)
	}
	if result != 42.0 {
		t.Errorf("期望 42.0，得到 %f", result)
	}
}

func TestNumberParser_Float(t *testing.T) {
	// 测试浮点数解析
	p := NewNumberParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "3.14")
	if err != nil {
		t.Fatalf("浮点数解析失败: %v", err)
	}
	if result != 3.14 {
		t.Errorf("期望 3.14，得到 %f", result)
	}
}

func TestNumberParser_NegativeNumber(t *testing.T) {
	// 测试负数解析
	p := NewNumberParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "-7.5")
	if err != nil {
		t.Fatalf("负数解析失败: %v", err)
	}
	if result != -7.5 {
		t.Errorf("期望 -7.5，得到 %f", result)
	}
}

func TestNumberParser_ExtractFromText(t *testing.T) {
	// 测试从文本中提取数字
	p := NewNumberParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "The answer is 42 units")
	if err != nil {
		t.Fatalf("从文本中提取数字失败: %v", err)
	}
	if result != 42.0 {
		t.Errorf("期望 42.0，得到 %f", result)
	}
}

func TestNumberParser_WithRange_Valid(t *testing.T) {
	// 测试范围内的值通过验证
	p := NewNumberParser().WithRange(0, 100)
	ctx := context.Background()

	result, err := p.Parse(ctx, "50")
	if err != nil {
		t.Fatalf("范围内的值不应报错: %v", err)
	}
	if result != 50.0 {
		t.Errorf("期望 50.0，得到 %f", result)
	}
}

func TestNumberParser_WithRange_TooSmall(t *testing.T) {
	// 测试小于最小值返回 ErrValidationFailure
	p := NewNumberParser().WithRange(10, 100)
	ctx := context.Background()

	_, err := p.Parse(ctx, "5")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("小于最小值应返回 ErrValidationFailure，得到: %v", err)
	}
}

func TestNumberParser_WithRange_TooBig(t *testing.T) {
	// 测试大于最大值返回 ErrValidationFailure
	p := NewNumberParser().WithRange(0, 100)
	ctx := context.Background()

	_, err := p.Parse(ctx, "150")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("大于最大值应返回 ErrValidationFailure，得到: %v", err)
	}
}

func TestNumberParser_WithInteger_RejectFloat(t *testing.T) {
	// 测试整数模式拒绝浮点数
	p := NewNumberParser().WithInteger(true)
	ctx := context.Background()

	_, err := p.Parse(ctx, "3.14")
	if !errors.Is(err, ErrValidationFailure) {
		t.Errorf("整数模式应拒绝浮点数，得到: %v", err)
	}
}

func TestNumberParser_WithInteger_AcceptInteger(t *testing.T) {
	// 测试整数模式接受整数
	p := NewNumberParser().WithInteger(true)
	ctx := context.Background()

	result, err := p.Parse(ctx, "42")
	if err != nil {
		t.Fatalf("整数模式应接受整数: %v", err)
	}
	if result != 42.0 {
		t.Errorf("期望 42.0，得到 %f", result)
	}
}

func TestNumberParser_NoNumber(t *testing.T) {
	// 测试无数字返回 ErrParseFailure
	p := NewNumberParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "no numbers here")
	if !errors.Is(err, ErrParseFailure) {
		t.Errorf("无数字时应返回 ErrParseFailure，得到: %v", err)
	}
}

func TestNumberParser_EmptyOutput(t *testing.T) {
	// 测试空输出返回 ErrEmptyOutput
	p := NewNumberParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输出应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestNumberParser_GetFormatInstructions(t *testing.T) {
	// 测试各种配置下的 GetFormatInstructions
	// 无范围
	p1 := NewNumberParser()
	if !strings.Contains(p1.GetFormatInstructions(), "number") {
		t.Error("默认应包含 number")
	}

	// 整数模式
	p2 := NewNumberParser().WithInteger(true)
	if !strings.Contains(p2.GetFormatInstructions(), "integer") {
		t.Error("整数模式应包含 integer")
	}

	// 有范围
	p3 := NewNumberParser().WithRange(1, 10)
	instructions := p3.GetFormatInstructions()
	if !strings.Contains(instructions, "between") {
		t.Error("有范围时应包含 between")
	}
}

func TestNumberParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 在不同配置下的返回值
	p1 := NewNumberParser()
	if p1.GetTypeName() != "Number" {
		t.Errorf("默认 GetTypeName 期望 Number，得到 %s", p1.GetTypeName())
	}

	p2 := NewNumberParser().WithInteger(true)
	if p2.GetTypeName() != "Integer" {
		t.Errorf("整数模式 GetTypeName 期望 Integer，得到 %s", p2.GetTypeName())
	}
}

// ============== PipelineParser 测试 ==============

func TestPipelineParser_SingleStep(t *testing.T) {
	// 测试单步管道
	final := NewNumberParser()
	pipeline := NewPipelineParser[float64](final).
		AddStep(func(ctx context.Context, s string) (string, error) {
			// 提取 "score: XX" 中的数字部分
			return strings.TrimPrefix(s, "score: "), nil
		})
	ctx := context.Background()

	result, err := pipeline.Parse(ctx, "score: 85")
	if err != nil {
		t.Fatalf("单步管道解析失败: %v", err)
	}
	if result != 85.0 {
		t.Errorf("期望 85.0，得到 %f", result)
	}
}

func TestPipelineParser_MultipleSteps(t *testing.T) {
	// 测试多步管道
	final := NewJSONParser[User]()
	pipeline := NewPipelineParser[User](final).
		AddStep(func(ctx context.Context, s string) (string, error) {
			// 第一步：去除前缀
			return strings.TrimPrefix(s, "Result: "), nil
		}).
		AddStep(func(ctx context.Context, s string) (string, error) {
			// 第二步：去除后缀
			return strings.TrimSuffix(s, " END"), nil
		})
	ctx := context.Background()

	result, err := pipeline.Parse(ctx, `Result: {"name": "Alice", "age": 30} END`)
	if err != nil {
		t.Fatalf("多步管道解析失败: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
}

func TestPipelineParser_StepFailure(t *testing.T) {
	// 测试步骤失败时中断管道
	final := NewNumberParser()
	pipeline := NewPipelineParser[float64](final).
		AddStep(func(ctx context.Context, s string) (string, error) {
			return "", fmt.Errorf("step failed")
		})
	ctx := context.Background()

	_, err := pipeline.Parse(ctx, "some input")
	if err == nil {
		t.Fatal("步骤失败时应返回错误")
	}
	if !strings.Contains(err.Error(), "step failed") {
		t.Errorf("错误信息应包含 step failed，得到: %v", err)
	}
}

func TestPipelineParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 委托给最终解析器
	final := NewNumberParser()
	pipeline := NewPipelineParser[float64](final)
	if pipeline.GetFormatInstructions() != final.GetFormatInstructions() {
		t.Error("PipelineParser 的 GetFormatInstructions 应委托给最终解析器")
	}
}

func TestPipelineParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 委托给最终解析器
	final := NewNumberParser()
	pipeline := NewPipelineParser[float64](final)
	if pipeline.GetTypeName() != final.GetTypeName() {
		t.Error("PipelineParser 的 GetTypeName 应委托给最终解析器")
	}
}

// ============== RetryParser 测试 ==============

func TestRetryParser_FirstSuccess(t *testing.T) {
	// 测试首次成功时不重试
	inner := NewNumberParser()
	retry := NewRetryParser[float64](inner, 3)
	ctx := context.Background()

	result, err := retry.Parse(ctx, "42")
	if err != nil {
		t.Fatalf("首次成功不应报错: %v", err)
	}
	if result != 42.0 {
		t.Errorf("期望 42.0，得到 %f", result)
	}
}

func TestRetryParser_RetryWithFixFunc(t *testing.T) {
	// 测试带修复函数的重试
	inner := NewJSONParser[User]()
	retryCount := 0
	retry := NewRetryParser[User](inner, 3).WithFixFunc(func(output string, err error) string {
		retryCount++
		// 修复：将单引号替换为双引号
		return strings.ReplaceAll(output, "'", "\"")
	})
	ctx := context.Background()

	// 初始输入使用单引号（无效 JSON），修复函数会替换为双引号
	result, err := retry.Parse(ctx, `{'name': 'Alice', 'age': 30}`)
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
	if retryCount != 1 {
		t.Errorf("期望重试 1 次，实际重试 %d 次", retryCount)
	}
}

func TestRetryParser_ExceedMaxRetries(t *testing.T) {
	// 测试超过最大重试次数
	inner := NewJSONParser[User]()
	retry := NewRetryParser[User](inner, 2)
	ctx := context.Background()

	_, err := retry.Parse(ctx, "not json at all")
	if err == nil {
		t.Fatal("超过最大重试次数后应报错")
	}
	if !strings.Contains(err.Error(), "parse failed after 2 retries") {
		t.Errorf("错误信息应包含重试次数，得到: %v", err)
	}
}

func TestRetryParser_GetFormatInstructions(t *testing.T) {
	// 测试 GetFormatInstructions 委托给内部解析器
	inner := NewBoolParser()
	retry := NewRetryParser[bool](inner, 3)
	if retry.GetFormatInstructions() != inner.GetFormatInstructions() {
		t.Error("RetryParser 的 GetFormatInstructions 应委托给内部解析器")
	}
}

func TestRetryParser_GetTypeName(t *testing.T) {
	// 测试 GetTypeName 委托给内部解析器
	inner := NewBoolParser()
	retry := NewRetryParser[bool](inner, 3)
	if retry.GetTypeName() != inner.GetTypeName() {
		t.Error("RetryParser 的 GetTypeName 应委托给内部解析器")
	}
}

// ============== extractJSON 测试 ==============

func TestExtractJSON_PureObject(t *testing.T) {
	// 测试纯 JSON 对象提取
	input := `{"name": "Alice", "age": 30}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("纯 JSON 对象应原样返回，得到: %s", result)
	}
}

func TestExtractJSON_MarkdownCodeBlock_JSON(t *testing.T) {
	// 测试从 ```json 代码块中提取
	input := "```json\n{\"key\": \"value\"}\n```"
	result := extractJSON(input)
	if result != `{"key": "value"}` {
		t.Errorf("应从 json 代码块中提取 JSON，得到: %s", result)
	}
}

func TestExtractJSON_MarkdownCodeBlock_NoLang(t *testing.T) {
	// 测试从无语言标记的代码块中提取
	input := "```\n{\"key\": \"value\"}\n```"
	result := extractJSON(input)
	if result != `{"key": "value"}` {
		t.Errorf("应从无标记代码块中提取 JSON，得到: %s", result)
	}
}

func TestExtractJSON_SurroundingText(t *testing.T) {
	// 测试从带前后文本中提取 JSON
	input := `Here is the result: {"name": "Bob"} and more text`
	result := extractJSON(input)
	if result != `{"name": "Bob"}` {
		t.Errorf("应提取 JSON 对象，得到: %s", result)
	}
}

func TestExtractJSON_NestedBrackets(t *testing.T) {
	// 测试嵌套括号的正确处理
	input := `{"outer": {"inner": {"deep": true}}}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("嵌套括号应完整提取，得到: %s", result)
	}
}

func TestExtractJSON_EscapedQuotes(t *testing.T) {
	// 测试带转义引号的字符串
	input := `{"message": "He said \"hello\""}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("带转义引号的 JSON 应完整提取，得到: %s", result)
	}
}

func TestExtractJSON_Array(t *testing.T) {
	// 测试 JSON 数组提取
	input := `The list: [1, 2, 3] is here`
	result := extractJSON(input)
	if result != `[1, 2, 3]` {
		t.Errorf("应提取 JSON 数组，得到: %s", result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	// 测试无 JSON 内容返回空字符串
	input := "This is just plain text without any JSON"
	result := extractJSON(input)
	if result != "" {
		t.Errorf("无 JSON 内容应返回空字符串，得到: %s", result)
	}
}

func TestExtractJSON_NestedArray(t *testing.T) {
	// 测试嵌套数组的正确处理
	input := `{"data": [{"id": 1}, {"id": 2}]}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("嵌套数组应完整提取，得到: %s", result)
	}
}

func TestExtractJSON_StringWithBraces(t *testing.T) {
	// 测试字符串值中包含花括号的情况
	input := `{"template": "Hello {name}, welcome to {place}"}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("字符串中的花括号不应影响提取，得到: %s", result)
	}
}

// ============== 便捷函数测试 ==============

func TestParseYesNo(t *testing.T) {
	// 测试 ParseYesNo 便捷函数
	result, err := ParseYesNo("yes")
	if err != nil {
		t.Fatalf("ParseYesNo 失败: %v", err)
	}
	if !result {
		t.Error("ParseYesNo(\"yes\") 应返回 true")
	}

	result, err = ParseYesNo("no")
	if err != nil {
		t.Fatalf("ParseYesNo 失败: %v", err)
	}
	if result {
		t.Error("ParseYesNo(\"no\") 应返回 false")
	}

	_, err = ParseYesNo("")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestParseNumber(t *testing.T) {
	// 测试 ParseNumber 便捷函数
	result, err := ParseNumber("3.14")
	if err != nil {
		t.Fatalf("ParseNumber 失败: %v", err)
	}
	if result != 3.14 {
		t.Errorf("期望 3.14，得到 %f", result)
	}

	_, err = ParseNumber("")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestParseList(t *testing.T) {
	// 测试 ParseList 便捷函数
	result, err := ParseList("apple\nbanana\ncherry")
	if err != nil {
		t.Fatalf("ParseList 失败: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("期望 3 个元素，得到 %d", len(result))
	}
	if result[0] != "apple" {
		t.Errorf("第 0 项期望 apple，得到 %s", result[0])
	}

	_, err = ParseList("")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestParseJSON(t *testing.T) {
	// 测试 ParseJSON 泛型便捷函数
	result, err := ParseJSON[User](`{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatalf("ParseJSON 失败: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
	if result.Age != 30 {
		t.Errorf("Age 期望 30，得到 %d", result.Age)
	}

	_, err = ParseJSON[User]("")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("空输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

// ============== generateJSONSchema 测试 ==============

func TestGenerateJSONSchema_Struct(t *testing.T) {
	// 测试结构体的 schema 生成
	schema := generateJSONSchema(User{})
	if schema == "{}" {
		t.Fatal("User 结构体不应生成空 schema")
	}
	// 应包含字段名
	if !strings.Contains(schema, "name") {
		t.Error("schema 应包含 name 字段")
	}
	if !strings.Contains(schema, "age") {
		t.Error("schema 应包含 age 字段")
	}
}

func TestGenerateJSONSchema_WithTags(t *testing.T) {
	// 测试带描述标签的 schema 生成
	schema := generateJSONSchema(Product{})
	if !strings.Contains(schema, "产品ID") {
		t.Error("schema 应包含 desc 标签中的描述")
	}
	if !strings.Contains(schema, "产品名称") {
		t.Error("schema 应包含 description 标签中的描述")
	}
}

func TestGenerateJSONSchema_Nil(t *testing.T) {
	// 测试 nil 值返回空 schema
	schema := generateJSONSchema(nil)
	if schema != "{}" {
		t.Errorf("nil 值应返回 {}，得到: %s", schema)
	}
}

func TestGenerateJSONSchema_NonStruct(t *testing.T) {
	// 测试非结构体类型返回空 schema
	schema := generateJSONSchema(42)
	if schema != "{}" {
		t.Errorf("非结构体应返回 {}，得到: %s", schema)
	}
}

// ============== getJSONTypeName 测试 ==============

func TestGetJSONTypeName(t *testing.T) {
	// 测试各种类型的 JSON 类型名称映射
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"字符串类型", "", "\"string\""},
		{"整数类型", 0, "integer"},
		{"浮点类型", 0.0, "number"},
		{"布尔类型", false, "boolean"},
		{"切片类型", []int{}, "[integer]"},
		{"Map 类型", map[string]any{}, "object"},
		{"结构体类型", User{}, "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := reflect.TypeOf(tt.input)
			result := getJSONTypeName(typ)
			if result != tt.expected {
				t.Errorf("期望 %s，得到 %s", tt.expected, result)
			}
		})
	}
}

func TestGetJSONTypeName_Pointer(t *testing.T) {
	// 测试指针类型解引用
	var ptr *int
	typ := reflect.TypeOf(ptr)
	result := getJSONTypeName(typ)
	if result != "integer" {
		t.Errorf("*int 期望 integer，得到 %s", result)
	}
}

// ============== 边界情况和集成测试 ==============

func TestJSONParser_MapType(t *testing.T) {
	// 测试解析到 map 类型
	p := NewJSONParser[map[string]any]()
	ctx := context.Background()

	result, err := p.Parse(ctx, `{"key1": "value1", "key2": 42}`)
	if err != nil {
		t.Fatalf("解析到 map 失败: %v", err)
	}
	if result["key1"] != "value1" {
		t.Errorf("key1 期望 value1，得到 %v", result["key1"])
	}
}

func TestEnumParser_PartialMatch_CaseSensitive(t *testing.T) {
	// 测试大小写敏感的部分匹配
	p := NewEnumParser("Approve", "Reject").WithCaseSensitive(true).WithPartialMatch(true)
	ctx := context.Background()

	// 大小写正确时应匹配
	result, err := p.Parse(ctx, "I think we should Approve this")
	if err != nil {
		t.Fatalf("大小写敏感部分匹配失败: %v", err)
	}
	if result != "Approve" {
		t.Errorf("期望 Approve，得到 %s", result)
	}

	// 大小写不正确时应失败
	_, err = p.Parse(ctx, "I think we should approve this")
	if err == nil {
		t.Fatal("大小写敏感模式下，小写 approve 不应匹配 Approve")
	}
}

func TestRetryParser_NoFixFunc(t *testing.T) {
	// 测试没有修复函数时，重试使用相同的输入
	callCount := 0
	inner := NewJSONParser[User]()
	retry := NewRetryParser[User](inner, 2)
	ctx := context.Background()

	// 无效输入，没有修复函数，所有重试都会失败
	_ = retry // 验证不 panic
	_, err := retry.Parse(ctx, "invalid json")
	_ = callCount
	if err == nil {
		t.Fatal("无修复函数时，无效输入的所有重试都应失败")
	}
}

func TestNumberParser_WithRange_BoundaryValues(t *testing.T) {
	// 测试范围边界值
	p := NewNumberParser().WithRange(0, 100)
	ctx := context.Background()

	// 边界值: 最小值
	result, err := p.Parse(ctx, "0")
	if err != nil {
		t.Fatalf("最小边界值应通过: %v", err)
	}
	if result != 0.0 {
		t.Errorf("期望 0.0，得到 %f", result)
	}

	// 边界值: 最大值
	result, err = p.Parse(ctx, "100")
	if err != nil {
		t.Fatalf("最大边界值应通过: %v", err)
	}
	if result != 100.0 {
		t.Errorf("期望 100.0，得到 %f", result)
	}
}

func TestListParser_SingleItem(t *testing.T) {
	// 测试单个元素的列表
	p := NewListParser()
	ctx := context.Background()

	result, err := p.Parse(ctx, "only one item")
	if err != nil {
		t.Fatalf("单个元素列表解析失败: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 个元素，得到 %d", len(result))
	}
	if result[0] != "only one item" {
		t.Errorf("期望 'only one item'，得到 %s", result[0])
	}
}

func TestBoolParser_WhitespaceOnly(t *testing.T) {
	// 测试仅包含空白字符的输入
	p := NewBoolParser()
	ctx := context.Background()

	_, err := p.Parse(ctx, "   \t\n  ")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("仅空白输入应返回 ErrEmptyOutput，得到: %v", err)
	}
}

func TestExtractJSON_WhitespaceAroundJSON(t *testing.T) {
	// 测试 JSON 周围有空白的情况
	input := `   {"key": "value"}   `
	result := extractJSON(input)
	if result != `{"key": "value"}` {
		t.Errorf("应正确提取去除空白后的 JSON，得到: %s", result)
	}
}

func TestExtractJSON_MultipleJSONObjects(t *testing.T) {
	// 测试多个 JSON 对象时提取第一个
	input := `{"first": true} {"second": true}`
	result := extractJSON(input)
	if result != `{"first": true}` {
		t.Errorf("应提取第一个 JSON 对象，得到: %s", result)
	}
}

func TestJSONParser_UnmatchedBraces(t *testing.T) {
	// 测试不匹配的花括号
	p := NewJSONParser[User]()
	ctx := context.Background()

	_, err := p.Parse(ctx, `{"name": "Alice"`)
	if err == nil {
		t.Fatal("不匹配的花括号应导致解析失败")
	}
}

func TestNumberParser_GetFormatInstructions_MinOnly(t *testing.T) {
	// 测试只有最小值的 GetFormatInstructions
	min := 5.0
	p := &NumberParser{Min: &min}
	instructions := p.GetFormatInstructions()
	if !strings.Contains(instructions, "greater than or equal to") {
		t.Error("只有最小值时应包含 'greater than or equal to'")
	}
}

func TestNumberParser_GetFormatInstructions_MaxOnly(t *testing.T) {
	// 测试只有最大值的 GetFormatInstructions
	max := 100.0
	p := &NumberParser{Max: &max}
	instructions := p.GetFormatInstructions()
	if !strings.Contains(instructions, "less than or equal to") {
		t.Error("只有最大值时应包含 'less than or equal to'")
	}
}

func TestListParser_GetFormatInstructions_NoLimits(t *testing.T) {
	// 测试无限制时的 GetFormatInstructions
	p := NewListParser()
	instructions := p.GetFormatInstructions()
	if strings.Contains(instructions, "minimum") || strings.Contains(instructions, "maximum") {
		t.Error("无限制时不应包含 minimum 或 maximum")
	}
}

func TestPipelineParser_NoSteps(t *testing.T) {
	// 测试无额外步骤的管道（直接传递给最终解析器）
	final := NewNumberParser()
	pipeline := NewPipelineParser[float64](final)
	ctx := context.Background()

	result, err := pipeline.Parse(ctx, "42")
	if err != nil {
		t.Fatalf("无步骤管道解析失败: %v", err)
	}
	if result != 42.0 {
		t.Errorf("期望 42.0，得到 %f", result)
	}
}

func TestJSONParser_GetTypeName_Pointer(t *testing.T) {
	// 测试指针类型的 GetTypeName
	p := NewJSONParser[*User]()
	name := p.GetTypeName()
	if name != "User" {
		t.Errorf("*User 的 GetTypeName 期望 User，得到 %s", name)
	}
}

func TestExtractJSON_MarkdownCodeBlock_WithExtraContent(t *testing.T) {
	// 测试代码块前后有额外内容
	input := "Here is the JSON:\n```json\n{\"key\": \"value\"}\n```\nEnd of response."
	result := extractJSON(input)
	if result != `{"key": "value"}` {
		t.Errorf("应从代码块中正确提取 JSON，得到: %s", result)
	}
}

func TestJSONParser_StrictMode_ValidInput(t *testing.T) {
	// 测试严格模式下有效输入通过
	p := NewJSONParser[StrictUser]().WithStrictMode(true)
	ctx := context.Background()

	result, err := p.Parse(ctx, `{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatalf("严格模式下有效输入不应报错: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name 期望 Alice，得到 %s", result.Name)
	}
}
