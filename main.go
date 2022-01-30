package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ZacJoffe/clipboard/xclip"
	"github.com/akamensky/argparse"
	"github.com/gen2brain/beeep"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type Config struct {
	UploadURL string            `yaml:"UploadURL"`
	Params    map[string]string `yaml:"Params"`
	Headers   map[string]string `yaml:"Headers"`
	Method    string            `yaml:"Method"`
	FormName  string            `yaml:"FormName"`
	URLFormat string            `yaml:"URLFormat"`
}

type ShareXConf struct {
	Version         string            `json:"Version"`
	Name            string            `json:"Name"`
	DestinationType string            `json:"DestinationType"`
	RequestMethod   string            `json:"RequestMethod"`
	RequestURL      string            `json:"RequestURL"`
	Parameters      map[string]string `json:"Parameters"`
	Headers         map[string]string `json:"Headers"`
	Body            string            `json:"Body"`
	FileFormName    string            `json:"FileFormName"`
	URL             string            `json:"URL"`
}

func main() {
	dependencyCheck()
	homeDir, _ := os.UserHomeDir()
	confDir, _ := os.UserHomeDir()
	confDir += "/.config/"

	parser := argparse.NewParser("Flameshot Uploader", "Run `flameshot gui -r | flameshotuploader -u` to use this tool.")

	pUpload := parser.Selector("u", "upload", []string{"image", "video", "gif"}, &argparse.Options{
		Required: false,
		Validate: nil,
		Help:     "Uploads the file from Stdin to the custom uploader. A file type of 'image', 'video', or 'gif' must be specified. Defaults to 'image'.",
		Default:  nil,
	})

	pImportConf := parser.String("i", "import-config", &argparse.Options{
		Required: false,
		Validate: nil,
		Help:     "Used for importing a ShareX config file. Expects a full path to the file.",
		Default:  nil,
	})

	pGetConf := parser.Flag("g", "get-config", &argparse.Options{
		Required: false,
		Validate: nil,
		Help:     "Show the current config/generate a blank one with this command",
		Default:  nil,
	})

	pUploadYouTube := parser.String("", "ytdl", &argparse.Options{
		Required: false,
		Validate: nil,
		Help:     "Upload any videos from YTDL supported sites to the uploader.",
		Default:  nil,
	})

	err := parser.Parse(os.Args)

	if err != nil {
	}

	if *pImportConf != "" {
		file, err := os.OpenFile(*pImportConf, os.O_RDONLY, 0775)
		if err != nil {

		} else {
			byteValue, _ := ioutil.ReadAll(file)

			var tmpConf ShareXConf
			err := json.Unmarshal(byteValue, &tmpConf)
			if err != nil {
				log.Fatalf("There was an error while parsing the ShareX config file: `%s`", err)
			}

			newConf := Config{
				UploadURL: tmpConf.RequestURL,
				Params:    tmpConf.Parameters,
				Headers:   tmpConf.Headers,
				Method:    tmpConf.RequestMethod,
				FormName:  tmpConf.FileFormName,
				URLFormat: tmpConf.URL,
			}

			marshal, err := yaml.Marshal(newConf)
			if err != nil {
				log.Fatalf("There was an error while parsing the new config file: `%s`", err)
			}

			err = os.MkdirAll(confDir, 0775)
			if err != nil {
				log.Fatalf("There was an error while creating the config directory: `%s`", err)
			}

			err = os.WriteFile(confDir+"FSUploader.yaml", marshal, 0775)
			if err != nil {
				log.Fatalf("There was an error while writing the config file: `%s`", err)
			}
			log.Printf("Config file successfully imported to %s\n", confDir+"FSUploader.yaml")
		}
	}

	if *pGetConf == true {
		config := loadConfig()

		params := ""
		headers := ""

		if len(config.Params) != 0 {
			for k, v := range config.Params {
				params += "\n - " + k + ": " + v + ""
			}
		}

		if len(config.Headers) != 0 {
			for k, v := range config.Headers {
				headers += "\n - " + k + ": " + v + ""
			}
		}

		fmt.Println("Flameshot Uploader config")
		fmt.Printf("URL: %s\nMethod: %s\nForm Name: %s\nParameters:%s\nHeaders:%s",
			config.UploadURL, config.Method, config.FormName, params, headers)
	}

	if *pUpload != "" || *pUploadYouTube != "" {
		config := loadConfig()

		var fileData *bufio.Reader

		url := config.UploadURL

		if len(config.Params) != 0 {
			var params []string
			for k, v := range config.Params {
				params = append(params, k+"="+v)
			}

			url += "?" + strings.Join(params, "&")
		}

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		if *pUploadYouTube == "" {
			fileData = loadStdin()

			if peek, _ := fileData.Peek(19); string(peek) == "screenshot aborted\n" {
				sendNotification("Image upload aborted.")
				os.Exit(1)
			}

			if *pUpload == "video" {
				part, _ := writer.CreateFormFile(config.FormName, "video.mp4")
				_, err = io.Copy(part, fileData)
				err = writer.Close()
			} else if *pUpload == "gif" {
				part, _ := writer.CreateFormFile(config.FormName, "video.gif")
				_, err = io.Copy(part, fileData)
				err = writer.Close()
			} else if *pUpload == "image" {
				part, _ := writer.CreateFormFile(config.FormName, "screenshot.png")
				_, err = io.Copy(part, fileData)
				err = writer.Close()
			} else {
				sendNotification("Error: No filetype specified, aborting.")
			}
		} else {
			ytdlCmd := exec.Command("yt-dlp", *pUploadYouTube, "--quiet", "--no-playlist", "--format", "mp4", "-o", homeDir+"/temp/FPUploader-video.mp4")

			stringBuilder := new(strings.Builder)
			ytdlCmd.Stdout = stringBuilder

			err = ytdlCmd.Run()
			if err != nil {
			}

			ytFilePath := homeDir + "/temp/FPUploader-video.mp4"

			if _, err := os.Stat(ytFilePath); !os.IsNotExist(err) {
				if err != nil {
					log.Fatal("Reading the YTDL video failed.")
				}

				readFile, err := os.ReadFile(ytFilePath)
				if err != nil {
				}

				buffer := new(bytes.Buffer)
				_, err = buffer.Write(readFile)
				if err != nil && err != io.EOF {
					log.Fatal("load stdin err: " + err.Error())
				}

				fileData = bufio.NewReader(buffer)

				part, _ := writer.CreateFormFile(config.FormName, "video.mp4")
				_, err = io.Copy(part, fileData)
				err = writer.Close()

				err = os.Remove(ytFilePath)
				if err != nil {
					return
				}
			}
		}

		request, _ := http.NewRequest(strings.ToUpper(config.Method), url, body)
		request.Header.Add("Content-Type", writer.FormDataContentType())

		if len(config.Headers) != 0 {
			for k, v := range config.Headers {
				request.Header.Add(k, v)
			}
		}

		client := &http.Client{}

		response, err := client.Do(request)
		if err != nil {
			sendNotification("There was an error while connecting to the server.")
			panic("")
		}
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(response.Body)

		bodyBytes, _ := ioutil.ReadAll(response.Body)

		if response.StatusCode == http.StatusOK {
			err := xclip.WriteText(string(bodyBytes))
			if err != nil {
				return
			}

			sendNotification("Upload successful!")
		} else {
			sendNotification("An error occurred whilst uploading the content")
			sendNotification(string(bodyBytes))
		}
	}
	parser.Help(os.Args)
}

func loadStdin() *bufio.Reader {
	buffer := new(bytes.Buffer)
	_, err := buffer.ReadFrom(os.Stdin)
	if err != nil {
		log.Fatal("load stdin err: " + err.Error())
	}

	return bufio.NewReader(buffer)
}

//func captureImg() []byte {
//	output, err := exec.Command("flameshot", "gui", "-r").Output()
//	if err != nil {
//		log.Println("capture img err: " + err.Error())
//	}
//	return output
//}

func dependencyCheck() {
	flameshotCheck, _ := exec.Command("flameshot", "-v").Output()
	xclipCheck, _ := exec.Command("xclip", "-version").Output()

	fmt.Println(string(flameshotCheck))
	fmt.Println(string(xclipCheck))
}

func sendNotification(message string) {
	err := beeep.Notify("Flameshot Uploader", message, "")
	if err != nil {
		return
	}
}

func setupConfig() {
	confDir, _ := os.UserHomeDir()
	confDir += "/.config/"

	err := os.MkdirAll(confDir, 0775)
	if err != nil {
		log.Fatalf("There was an error while creating the config directory: '%s'", err)
	}
	if _, err := os.Stat(confDir + "FSUploader.yaml"); os.IsNotExist(err) {
		confTmp, _ := json.Marshal(Config{
			UploadURL: "",
			Params:    map[string]string{},
			Headers:   map[string]string{},
			Method:    "POST",
			FormName:  "img",
		})

		err := os.WriteFile(confDir+"FSUploader.yaml", confTmp, 0775)
		if err != nil {
			log.Fatalf("There was an error while writing to the config file: '%s'", err)
		} else {
			log.Printf("Blank config file created at: %s\nPlease re-run the script.", confDir+"FSUploader.yaml")
			os.Exit(1)
		}
	}
}

func loadConfig() Config {
	confDir, _ := os.UserHomeDir()
	confDir += "/.config/"

	if fi, err := os.Stat(confDir + "FSUploader.yaml"); err == nil {
		var config Config
		readFile, err := os.ReadFile(confDir + fi.Name())

		if err != nil {
			log.Fatalf("There was an error while reading the config file: '%s'", err)
		}
		err = yaml.Unmarshal(readFile, &config)
		if err != nil {
			log.Fatalf("There was an error while parsing the config file: '%s'", err)
		}
		log.Println("config loaded!")
		return config
	}
	log.Println("Config file not found, creating one now...")
	setupConfig()
	return Config{}
}
