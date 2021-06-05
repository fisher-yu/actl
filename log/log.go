package log

import (
	"fmt"

	"github.com/fisher-yu/actl/util"
)

func Debug(message interface{}) {
	fmt.Println(util.Cyan(fmt.Sprint(message)))
}

func Info(message interface{}) {
	fmt.Println(util.Blue(fmt.Sprint(message)))
}

func Warn(message interface{}) {
	fmt.Println(util.Yellow(fmt.Sprint(message)))
}

func Success(message interface{}) {
	fmt.Println(util.Green(fmt.Sprint(message)))
}

func Error(message interface{}) {
	fmt.Println(util.Red(fmt.Sprint(message)))
}

func Fatal(message interface{}) {
	fmt.Println(util.Magenta(fmt.Sprint(message)))
}
