package db2

import (
	"errors"
	"fmt"
	logrus "github.com/sirupsen/logrus"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var log *logrus.Logger

func init() {
	if log == nil {
		log = logrus.New()
		log.SetLevel(logrus.PanicLevel)
	}
}
func LogRegister(logger *logrus.Logger) {
	log = logger
}

//db2在调用db2命令的时候每一个session就第一次调用比较慢，后续都较快，因为可以每次都获取结果，也可以放到一起批量生成结果进行调用
//所有结果中不可用有空结果，不可以有换行符

var mon_get_start_flag = "_start"
var mon_get_end_flag = "_end"
var mon_get_rep = ";"
var timestamp_short_form = "2006-01-02-15.04.05.000000"

func GetCurInstanceName() string {
	u, _ := user.Current()
	return u.Name
}

type MataData struct {
	tabname    string
	start_flag string
	end_flag   string
	rep        string
}

//解析结构体指针，生成字段信息
func reflectMonGet(ptr interface{}) string {
	colnameList := make([]string, 0)
	numFields := reflect.TypeOf(ptr).Elem().NumField()
	for i := 0; i < numFields; i++ {
		if v, ok := reflect.TypeOf(ptr).Elem().Field(i).Tag.Lookup("column"); ok {
			colnameList = append(colnameList, v)
		}
	}
	return strings.Join(colnameList, ",")
}

func genSql(m interface{}) string {
	start := reflect.ValueOf(m).Elem().FieldByName("start_flag").String()
	end := reflect.ValueOf(m).Elem().FieldByName("end_flag").String()
	rep := reflect.ValueOf(m).Elem().FieldByName("rep").String()
	tabname := reflect.ValueOf(m).Elem().FieldByName("tabname").String()
	start_str := fmt.Sprintf("!echo \"%s\"%s\n", start, rep)
	end_str := fmt.Sprintf("!echo \"%s\"%s\n", end, rep)
	return fmt.Sprintf("%sselect %s from %s with ur%s\n%s", start_str, reflectMonGet(m), tabname, rep, end_str)

}

//将一行记录渲染到一个结构体中,以空格为分隔符，如果文本数量大于结构体字段数量，那么把所有剩余文本列赋予最后一个结构体属性中
func renderStruct(ptr interface{}, str string) error {
	fields := strings.Fields(strings.TrimSpace(str))
	numFields := len(fields)
	ptr_numFields := reflect.TypeOf(ptr).Elem().NumField()
	//记录包含column tag的字段
	ptr_fields_nbr := make([]int, 0)
	for i := 0; i < ptr_numFields; i++ {
		if _, ok := reflect.TypeOf(ptr).Elem().Field(i).Tag.Lookup("column"); ok {
			ptr_fields_nbr = append(ptr_fields_nbr, i)
		}
	}
	//查看结构体中包含column的字段是否和ptr_fields_nbr一样多
	if numFields < len(ptr_fields_nbr) {
		msg := fmt.Sprintf("name:%s,行中列数小于结构体中字段个数,列数：%d,结构体字段数:%d,行内容:%s\n",
			reflect.TypeOf(ptr).Elem().Name(), numFields, len(ptr_fields_nbr), strings.TrimSpace(str))
		log.Debug(msg)
		return errors.New(msg)
	} else if numFields > len(ptr_fields_nbr) {
		fields = append(fields[:len(ptr_fields_nbr)-1], strings.Join(fields[len(ptr_fields_nbr)-1:numFields-1], " "))
		numFields = len(fields)
	}
	for i := 0; i < numFields; i++ {
		//查看ptr中字段的类型看是否需要进行转换
		v_type := reflect.TypeOf(ptr).Elem().Field(ptr_fields_nbr[i]).Type.String()
		v := reflect.ValueOf(ptr).Elem().Field(ptr_fields_nbr[i])
		if v.CanSet() {
			switch v_type {
			case "int":
				if fields[i] == "-" {
					v.SetInt(-1)
				} else {
					val, err := strconv.Atoi(fields[i])
					if err != nil {
						return err
					}
					v.SetInt(int64(val))
				}

			case "string":
				if fields[i] == "-" {
					v.SetString("")
				} else {
					v.SetString(fields[i])
				}

			case "time.Time":
				//如果是时间格式则进行时间格式转换
				if fields[i] == "-" {
					t := time.Unix(0, 0)
					v.Set(reflect.ValueOf(t))
				} else {
					t, err := time.Parse(timestamp_short_form, fields[i])
					if err != nil {
						return err
					}
					v.Set(reflect.ValueOf(t))
				}
			default:
				return errors.New("Cannot set value,type:" + v_type)
			}
		}
	}
	return nil
}
