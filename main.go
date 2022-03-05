package main

import (
	"embed"
	"fmt"
	"html/template"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"voucher/internal/database"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code93"
	"github.com/gin-gonic/gin"
	"github.com/rs/xid"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	APP_NAME = "voucherer"
)

var (
	//go:embed assets
	assets embed.FS
)

func main() {
	pflag.StringP("port", "p", "8080", "Port to listen on")
	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)

	viper.SetDefault("open_browser_automatically", true)

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.SafeWriteConfig()

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("failed to read config file: %w", err))
	}

	db, err := database.New("database.sqlite")
	if err != nil {
		panic(fmt.Errorf("failed to create sqlite database: %w", err))
	}

	logFile, err := os.Create("log.txt")
	if err != nil {
		panic(fmt.Errorf("failed to create log file: %w", err))
	}

	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.MultiWriter(os.Stdout, logFile)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, logFile)

	log := func(a ...interface{}) {
		fmt.Fprintln(gin.DefaultWriter, a...)
	}

	logError := func(a ...interface{}) {
		fmt.Fprintln(gin.DefaultErrorWriter, a...)
	}

	r := gin.Default()

	r.SetHTMLTemplate(
		template.Must(
			template.New("").ParseFS(assets, "assets/templates/*"),
		),
	)

	redirectHome := func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, `<meta http-equiv="Refresh" content="0; url='/'" />`)
	}

	r.GET("/", func(c *gin.Context) {
		vouchers, err := db.GetVouchers()
		if err != nil {
			logError("failed to get vouchers from database:", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"appName":  APP_NAME,
			"vouchers": vouchers,
		})
	})

	r.GET("/about", func(c *gin.Context) {
		c.HTML(http.StatusOK, "about.tmpl", gin.H{
			"appName": APP_NAME,
		})
	})

	r.POST("/delete", func(c *gin.Context) {
		if err := c.Request.ParseForm(); err != nil {
			logError("failed to parse form:", err)
			c.Status(http.StatusBadRequest)
			return
		}
		codes := c.Request.Form["code"]
		if err := db.DeleteVouchers(codes...); err != nil {
			logError("failed to delete voucher:", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		redirectHome(c)
	})

	r.POST("/voucher", func(c *gin.Context) {
		code := xid.New()
		if err := db.CreateVoucher(code.String()); err != nil {
			logError("failed to create voucher:", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.HTML(http.StatusOK, "voucher.tmpl", gin.H{
			"appName": APP_NAME,
			"code":    code,
		})
	})

	r.POST("/redeem", func(c *gin.Context) {
		code := c.Request.FormValue("code")
		redeemStatus, err := db.RedeemVoucher(code)
		if err != nil {
			logError("failed to redeem voucher:", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		switch redeemStatus {
		case database.RedeemStatusSuccess:
			redirectHome(c)
			return
		case database.RedeemStatusNotExists:
			c.HTML(http.StatusNotFound, "redeem_error.tmpl", gin.H{
				"error": "does not exist",
				"code":  code,
			})
			return
		case database.RedeemStatusRedeemed:
			c.HTML(http.StatusNotFound, "redeem_error.tmpl", gin.H{
				"error": "already redeemed",
				"code":  code,
			})
			return
		case database.RedeemStatusError:
			fallthrough
		default:
			c.Status(http.StatusInternalServerError)
			return
		}
	})

	r.GET("/barcode/:code", func(c *gin.Context) {
		code := c.Params.ByName("code")
		b, err := code93.Encode(code, true, true)
		if err != nil {
			logError("error encoding code93 barcode: %w", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		b, err = barcode.Scale(b, 360, 60)
		if err != nil {
			logError("error scaling barcode: %w", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		if err := png.Encode(c.Writer, b); err != nil {
			logError("failed encoding barcode png: %w", err)
			c.Status(http.StatusInternalServerError)
			return
		}
	})

	r.StaticFS("/static", http.FS(assets))

	addr := fmt.Sprintf("localhost:%s", viper.GetString("port"))
	log("listening on", addr)

	if viper.GetBool("open_browser_automatically") {
		openInBrowser(fmt.Sprintf("http://%s", addr))
	}

	r.Run(addr)
}

func openInBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
