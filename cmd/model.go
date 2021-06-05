package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/fisher-yu/actl/log"
	tpls "github.com/fisher-yu/actl/tpl"
	"github.com/fisher-yu/actl/util"

	"xorm.io/xorm/schemas"

	"xorm.io/core"

	_ "github.com/go-sql-driver/mysql"
	"xorm.io/xorm"

	"github.com/spf13/viper"

	_ "go/format"

	"github.com/spf13/cobra"
)

const (
	DefaultDir  = "./app/models"
	DefaultConf = "./conf/app.toml"
)

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	integerKind
	stringKind
	uintKind
)

var (
	Table  string
	Dir    string
	Config string
)

var (
	errBadComparisonType = errors.New("invalid type for comparison")
	errBadComparison     = errors.New("incompatible types for comparison")
	errNoComparison      = errors.New("missing argument for comparison")
)

var (
	modelCmd = &cobra.Command{
		Use:   "model [OPTIONS] ...",
		Short: "Model Generator",
		Long:  `model generator for xorm`,
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}
)

// 初始化 ，先运行init 再运行run
func init() {
	modelCmd.Flags().StringVarP(&Table, "table", "t", "",
		"table name，multiple names can be separated by ','.")
	modelCmd.Flags().StringVarP(&Dir, "dir", "d", DefaultDir,
		"models dir")
	modelCmd.Flags().StringVarP(&Config, "config", "c",
		DefaultConf, "mysql config")

	rootCmd.AddCommand(modelCmd)
}

func run() {
	vp := viper.New()
	if res, reason := checkRequired(vp); !res {
		printError(reason)
	}
	eg, err := getEngineGroup(vp)
	if err != nil {
		printError(err.Error())
	}
	if err = eg.Ping(); err != nil {
		printError(err.Error())
	}

	tables, err := eg.DBMetas()
	if err != nil {
		printError(err.Error())
	}

	var ts []string
	for _, table := range tables {
		ts = append(ts, table.Name)
	}
	its := strings.Split(Table, ",")
	for _, it := range its {
		if !util.InSlice(ts, it) {
			printError(it + " table not found in the database")
		}
	}

	fmt.Println()
	for _, table := range tables {
		if !util.InSlice(its, table.Name) {
			continue
		}
		imports := genImports(table)
		tplContent, err := genTplContent(table, imports)
		if err != nil {
			printError(err.Error())
		}

		err = genModel(table, tplContent)
		if err != nil {
			printError(err.Error())
		}
		modelFile := path.Join(Dir, table.Name+".go")
		log.Success("create " + modelFile)

	}
	fmt.Println()

}

// 检测参数，mysql配置
func checkRequired(vp *viper.Viper) (bool, string) {
	// 检测是否指定了数据表
	if len(Table) == 0 {
		return false, "table name cannot be blank"
	}

	fi, err := os.Stat(Config)
	if err != nil {
		return false, "config file：" + Config + " not found"
	}
	if fi.IsDir() {
		return false, "config file cannot be a dir"
	}

	// 检查配置文件是否存在
	i := strings.LastIndex(Config, "/")
	di := strings.LastIndex(Config, ".")
	cfgName := Config[i+1 : di]
	cfgPath := Config[0:i]
	vp.SetConfigName(cfgName)
	vp.AddConfigPath(cfgPath)
	err = vp.ReadInConfig()

	if err != nil {
		return false, "config file：" + Config + " not found"
	}

	return true, ""
}

// 打印错误信息
func printError(message string) {
	log.Debug("\nModels not generated. Please fix the following errors:\n")
	log.Error(message + "\n")
	fmt.Println("See \"actl help model\"")
	os.Exit(0)
}

// 获取MySQL引擎
func getEngineGroup(vp *viper.Viper) (*xorm.EngineGroup, error) {
	// 检测MySQL配置项是否存在
	conf := vp.Get("mysql")
	if conf == nil {
		return nil, errors.New("mysql item not found in config")
	}

	// 检测MySQL配置是否正确
	cfg, b := conf.(map[string]interface{})
	if !b {
		return nil, errors.New("parse mysql config error")
	}

	if cfg["host"] == nil {
		return nil, errors.New("mysql.host is not found")
	}
	if cfg["user"] == nil {
		return nil, errors.New("mysql.user is not found")
	}
	if cfg["password"] == nil {
		return nil, errors.New("mysql.password is not found")
	}
	if cfg["database"] == nil {
		return nil, errors.New("mysql.database is not found")
	}

	var conns []string
	hosts := strings.Split(cfg["host"].(string), ",")
	for _, h := range hosts {
		conns = append(conns, fmt.Sprintf(
			"%s:%s@tcp(%s)/%s?charset=utf8&parseTime=True&loc=Local",
			cfg["user"], cfg["password"], h, cfg["database"]))
	}

	return xorm.NewEngineGroup("mysql", conns)
}

// 生成模板中的imports
func genImports(table *schemas.Table) map[string]string {
	imports := make(map[string]string)

	for _, col := range table.Columns() {
		if typeString(col) == "time.Time" {
			imports["time"] = "time"
		}
	}

	return imports
}

// 获取模板解析后的内容
func genTplContent(table *schemas.Table, imports map[string]string) (string, error) {
	type tplTable struct {
		Tables  []*schemas.Table
		Imports map[string]string
		Models  string
	}

	mapper := &core.SnakeMapper{}
	funcMap := template.FuncMap{"Mapper": mapper.Table2Obj,
		"Type":       typeString,
		"Tag":        tag,
		"UnTitle":    unTitle,
		"gt":         gt,
		"getCol":     getCol,
		"UpperTitle": upTitle,
	}

	t := template.New("go.tpl")
	t.Funcs(funcMap)

	tpl, err := t.Parse(tpls.ModelTpl)
	if err != nil {
		return "", err
	}
	tables := []*schemas.Table{table}
	newBuf := bytes.NewBufferString("")
	tb := &tplTable{Tables: tables, Imports: imports, Models: "models"}
	err = tpl.Execute(newBuf, tb)
	if err != nil {
		return "", err
	}

	tplContent, err := ioutil.ReadAll(newBuf)
	if err != nil {
		return "", err
	}

	return string(tplContent), nil
}

// 生成Model
func genModel(table *schemas.Table, content string) error {
	if !dirExists(Dir) {
		if err := os.MkdirAll(Dir, os.ModePerm); err != nil {
			return err
		}
	}

	f, err := os.Create(path.Join(Dir, table.Name+".go"))
	if err != nil {
		return err
	}
	defer f.Close()

	source, err := formatCode(content)
	if err != nil {
		return err
	}

	_, err = f.WriteString(source)
	if err != nil {
		return err
	}

	return nil

}

// 检测Dir是否存在
func dirExists(dir string) bool {
	d, e := os.Stat(dir)
	switch {
	case e != nil:
		return false
	case !d.IsDir():
		return false
	}

	return true
}

// 格式化代码
func formatCode(src string) (string, error) {
	source, err := format.Source([]byte(src))
	if err != nil {
		return "", err
	}
	return string(source), nil
}

func typeString(col *schemas.Column) string {
	st := col.SQLType
	t := schemas.SQLType2Type(st)
	s := t.String()
	if s == "[]uint8" {
		return "[]byte"
	}
	return s
}

func tag(table *schemas.Table, col *schemas.Column) string {
	isNameId := col.Name == "Id"
	isIdPk := isNameId && typeString(col) == "int64"

	var res []string
	if !col.Nullable {
		if !isIdPk {
			res = append(res, "not null")
		}
	}
	if col.IsPrimaryKey {
		res = append(res, "pk")
	}
	if col.Default != "" {
		res = append(res, "default "+col.Default)
	}
	if col.IsAutoIncrement {
		res = append(res, "autoincr")
	}

	if col.SQLType.IsTime() && col.Name == "created_at" {
		res = append(res, "created")
	}

	if col.SQLType.IsTime() && col.Name == "updated_at" {
		res = append(res, "updated")
	}

	if col.SQLType.IsTime() && col.Name == "deleted_at" {
		res = append(res, "deleted")
	}

	if col.Comment != "" {
		res = append(res, fmt.Sprintf("comment('%s')", col.Comment))
	}

	names := make([]string, 0, len(col.Indexes))
	for name := range col.Indexes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		index := table.Indexes[name]
		var uistr string
		if index.Type == core.UniqueType {
			uistr = "unique"
		} else if index.Type == core.IndexType {
			uistr = "index"
		}
		if len(index.Cols) > 1 {
			uistr += "(" + index.Name + ")"
		}
		res = append(res, uistr)
	}

	nstr := col.SQLType.Name
	if col.Length != 0 {
		if col.Length2 != 0 {
			nstr += fmt.Sprintf("(%v,%v)", col.Length, col.Length2)
		} else {
			nstr += fmt.Sprintf("(%v)", col.Length)
		}
	} else if len(col.EnumOptions) > 0 { //enum
		nstr += "("
		opts := ""

		enumOptions := make([]string, 0, len(col.EnumOptions))
		for enumOption := range col.EnumOptions {
			enumOptions = append(enumOptions, enumOption)
		}
		sort.Strings(enumOptions)

		for _, v := range enumOptions {
			opts += fmt.Sprintf(",'%v'", v)
		}
		nstr += strings.TrimLeft(opts, ",")
		nstr += ")"
	} else if len(col.SetOptions) > 0 { //enum
		nstr += "("
		opts := ""

		setOptions := make([]string, 0, len(col.SetOptions))
		for setOption := range col.SetOptions {
			setOptions = append(setOptions, setOption)
		}
		sort.Strings(setOptions)

		for _, v := range setOptions {
			opts += fmt.Sprintf(",'%v'", v)
		}
		nstr += strings.TrimLeft(opts, ",")
		nstr += ")"
	}
	res = append(res, nstr)

	var tags []string
	tags = append(tags, "json:\""+col.Name+"\"")
	if len(res) > 0 {
		tags = append(tags, "xorm:\""+strings.Join(res, " ")+"\"")
	}

	if len(tags) > 0 {
		return "`" + (strings.Join(tags, " ")) + "`"
	} else {
		return ""
	}
}

func unTitle(src string) string {
	if src == "" {
		return ""
	}

	if len(src) == 1 {
		return strings.ToLower(string(src[0]))
	} else {
		return strings.ToLower(string(src[0])) + src[1:]
	}
}

func upTitle(src string) string {
	if src == "" {
		return ""
	}

	return strings.ToUpper(src)
}

// gt evaluates the comparison a > b.
func gt(arg1, arg2 interface{}) (bool, error) {
	// > is the inverse of <=.
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}

	return !lessOrEqual, nil
}

// lt evaluates the comparison a < b.
func lt(arg1, arg2 interface{}) (bool, error) {
	v1 := reflect.ValueOf(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}

	v2 := reflect.ValueOf(arg2)
	k2, err := basicKind(v2)
	if err != nil {
		return false, err
	}

	if k1 != k2 {
		return false, errBadComparison
	}

	truth := false
	switch k1 {
	case boolKind, complexKind:
		return false, errBadComparisonType
	case floatKind:
		truth = v1.Float() < v2.Float()
	case intKind:
		truth = v1.Int() < v2.Int()
	case stringKind:
		truth = v1.String() < v2.String()
	case uintKind:
		truth = v1.Uint() < v2.Uint()
	default:
		panic("invalid kind")
	}

	return truth, nil
}

// le evaluates the comparison <= b.
func le(arg1, arg2 interface{}) (bool, error) {
	// <= is < or ==.
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}

	return eq(arg1, arg2)
}

// eq evaluates the comparison a == b || a == c || ...
func eq(arg1 interface{}, arg2 ...interface{}) (bool, error) {
	v1 := reflect.ValueOf(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}

	if len(arg2) == 0 {
		return false, errNoComparison
	}

	for _, arg := range arg2 {
		v2 := reflect.ValueOf(arg)
		k2, err := basicKind(v2)
		if err != nil {
			return false, err
		}

		if k1 != k2 {
			return false, errBadComparison
		}

		truth := false
		switch k1 {
		case boolKind:
			truth = v1.Bool() == v2.Bool()
		case complexKind:
			truth = v1.Complex() == v2.Complex()
		case floatKind:
			truth = v1.Float() == v2.Float()
		case intKind:
			truth = v1.Int() == v2.Int()
		case stringKind:
			truth = v1.String() == v2.String()
		case uintKind:
			truth = v1.Uint() == v2.Uint()
		default:
			panic("invalid kind")
		}

		if truth {
			return true, nil
		}
	}

	return false, nil
}

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	}
	return invalidKind, errBadComparisonType
}

func getCol(cols map[string]*core.Column, name string) *core.Column {
	return cols[strings.ToLower(name)]
}
