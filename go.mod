module github.com/bartdeboer/ctgbot

go 1.24.0

toolchain go1.24.11

require (
	github.com/bartdeboer/go-clir v0.0.5
	github.com/bartdeboer/go-clistate v0.0.6
	github.com/go-telegram/bot v1.17.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/bartdeboer/words v0.0.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/text v0.20.0 // indirect
)

// replace github.com/bartdeboer/go-clir => ../go-clir

// replace github.com/bartdeboer/go-clistate => ../go-clistate
