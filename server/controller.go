package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"time"

	"github.com/gorilla/mux"
)

func returnError(title string, err string, trace string, code int, writer http.ResponseWriter) {
	if Config.Mail.ErrorTo != "" && title != "Not found" {
		log.Println("send error email...")
		go SendError(title+": "+err, trace)
		trace = "We have received the technical details of this error and will look into it."
	}
	WriteHtmlWithStatus(ErrorView(title, err, trace), code, writer)
}

func httpError(writer http.ResponseWriter, code int, message string, error error) {
	log.Println("HTTP ERROR", code, message, error)
	title := "Error: " + message
	if code == 404 {
		title = "Not found"
	}
	returnError(title, message, error.Error(), code, writer)
}

func redirectToLastVersion(writer http.ResponseWriter, packageName string) {
	latestVersion, err := DbGetPackageLatestVersion(packageName)
	if err != nil {
		packageInfo, err := GetPackageInfo(packageName)
		if err != nil {
			httpError(writer, http.StatusNotFound, "could not get package "+packageName, err)
			return
		}
		latestVersion = packageInfo.DistTags.Latest
	}
	writer.Header().Set("Location", "/npm/"+packageName+"/"+latestVersion)
	writer.WriteHeader(http.StatusFound)
}

func packageHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	ns := vars["ns"]
	name := vars["name"]
	if ns != "" {
		name = ns + "/" + name
	}
	redirectToLastVersion(writer, name)
}

func versionHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	ns := vars["ns"]
	name := vars["name"]
	versionRaw := vars["version"]
	if ns != "" {
		name = ns + "/" + name
	}
	version, err := GetVersion(name, versionRaw)
	if err == TimeoutError {
		WriteHtml(WaitView(name), writer)
		return
	}
	if err != nil {
		httpError(writer, http.StatusNotFound, "could not get dependencies for package "+name+" "+versionRaw, err)
		return
	}
	WriteHtml(VersionView(version), writer)
}

func goHandler(writer http.ResponseWriter, request *http.Request) {
	name := request.URL.Query().Get("package")
	redirectToLastVersion(writer, name)
}

func pageHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	path := vars["path"]
	page, err := GetPage(path)
	if err != nil {
		httpError(writer, http.StatusNotFound, "could not get page "+path, err)
		return
	}
	WriteHtml(PageView(page), writer)
}

func homeHandler(writer http.ResponseWriter, request *http.Request) {
	WriteHtml(HomeView(), writer)
}

const SAFE_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// about 64 bits of entropy = about 11 chars
func randId(n int) string {
	var id []byte
	for i := 0; i < n; i++ {
		id = append(id, SAFE_CHARS[rand.Intn(62)])
	}
	return string(id)
}

const MAX_UPLOAD_SIZE = 1000000

func uploadHandler(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, MAX_UPLOAD_SIZE)
	if err := request.ParseMultipartForm(MAX_UPLOAD_SIZE); err != nil {
		httpError(writer, http.StatusBadRequest, "the uploaded file is >1MB", err)
		return
	}
	file, _, err := request.FormFile("file")
	if err != nil {
		httpError(writer, http.StatusBadRequest, "could not get uploaded file from form", err)
		return
	}
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		httpError(writer, http.StatusBadRequest, "could not read uploaded file", err)
		return
	}
	var versionInfo VersionInfo
	if err := json.Unmarshal(bytes, &versionInfo); err != nil {
		httpError(writer, http.StatusBadRequest, "could not parse uploaded file", err)
		return
	}

	version := NewVersion(versionInfo, time.Now())
	id := randId(11)
	if err := DbPutFile(id, version); err != nil {
		httpError(writer, http.StatusBadRequest, "could not store file", err)
		return
	}

	writer.Header().Set("Location", "/file/"+id)
	writer.WriteHeader(http.StatusMovedPermanently)
}

func fileHandler(writer http.ResponseWriter, request *http.Request) {
	id := mux.Vars(request)["id"]
	version, err := GetFile(id)
	if err == TimeoutError {
		WriteHtml(WaitView("your package.json"), writer)
		return
	}
	if err != nil {
		httpError(writer, http.StatusNotFound, "could not get dependencies for file "+id, err)
		return
	}
	WriteHtml(VersionView(version), writer)
}

func writePanic(writer http.ResponseWriter, errObj interface{}, buf []byte) {
	err := fmt.Sprint(errObj)

	log.Println(err, string(buf))

	returnError("Internal Server Error", err, string(buf), http.StatusInternalServerError, writer)
}

func PanicRecovery(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 16384)
				n := runtime.Stack(buf, false)
				buf = buf[:n]

				writePanic(w, err, buf)
			}
		}()

		handler.ServeHTTP(w, r)
	})
}

func Serve(publicFs fs.FS) {
	r := mux.NewRouter()
	r.HandleFunc("/npm/{name:[\\w\\-.]+}", packageHandler)
	r.HandleFunc("/npm/{ns:@[\\w\\-]+}/{name:[\\w\\-.]+}", packageHandler)
	r.HandleFunc("/npm/{name:[\\w\\-.]+}/{version:\\d.*}", versionHandler)
	r.HandleFunc("/npm/{ns:@[\\w\\-]+}/{name:[\\w\\-.]+}/{version:\\d.*}", versionHandler)

	r.HandleFunc("/upload", uploadHandler)
	r.HandleFunc("/file/{id}", fileHandler)
	r.HandleFunc("/go", goHandler)

	r.HandleFunc("/pages/{path:.*}", pageHandler)
	r.HandleFunc("/error", func(writer http.ResponseWriter, r *http.Request) { log.Panicln("test panic") })
	r.HandleFunc("/", homeHandler)

	r.PathPrefix("/").Handler(http.FileServer(http.FS(publicFs)))

	r.Use(PanicRecovery)

	listenAddr := fmt.Sprintf("localhost:%d", Config.Server.Port)
	server := http.Server{Addr: listenAddr, Handler: r}
	log.Println("start listening at http://" + listenAddr + "...")
	err := server.ListenAndServe()
	if err != nil {
		log.Panicln("could not start server", err)
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
