package web

import (
	"html/template"
	"net/http"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
)

func adminIndex(w http.ResponseWriter, r *http.Request) {

	tmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles(configs.GetFilePathsConfig().AdminHtml.String()+"/_header.html", configs.GetFilePathsConfig().AdminHtml.String()+"/index.html", configs.GetFilePathsConfig().AdminHtml.String()+"/_footer.html")
	if err != nil {
		mudlog.Error("HTML ERROR", "error", err)
	}

	tmpl.Execute(w, nil)

}

func adminViewConfig(w http.ResponseWriter, r *http.Request) {

	adminHtml := configs.GetFilePathsConfig().AdminHtml.String()

	tmpl, err := template.New("viewconfig.html").Funcs(funcMap).ParseFiles(
		adminHtml+"/_header.html",
		adminHtml+"/viewconfig.html",
		adminHtml+"/_footer.html",
	)
	if err != nil {
		mudlog.Error("HTML ERROR", "error", err)
		http.Error(w, "Error parsing template files", http.StatusInternalServerError)
		return
	}

	templateData := map[string]any{
		"CONFIG": configs.GetConfig(),
	}

	if err := tmpl.Execute(w, templateData); err != nil {
		mudlog.Error("HTML ERROR", "action", "Execute", "error", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}
