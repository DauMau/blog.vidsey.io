package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2/facebook"

	"golang.org/x/oauth2"
)

var config = oauth2.Config{
	ClientID:     os.Getenv("FB_APP_ID"),
	ClientSecret: os.Getenv("FB_APP_SECRET"),
	Endpoint:     facebook.Endpoint,
	RedirectURL:  "http://localhost:8080/callback",
	Scopes:       []string{"email", "public_profile", "user_photos"},
}

func main() {
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/profile", profileHandler)
	http.ListenAndServe(":8080", nil)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, config.AuthCodeURL("test-state"), http.StatusTemporaryRedirect)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	const callbackTemplate = `
	<html>
		<head><meta http-equiv="refresh" content="5; url=/profile"><head>
		<body>
			<p><b>Token:</b> <code>%s</code></p>
			<p>You will be redirected in 5 seconds to 
				<a href="profile">profile</a>
			</p>
		</body>
	</html>`
	query := r.URL.Query()
	if message := query.Get("error"); message != "" {
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	token, err := config.Exchange(oauth2.NoContext, query.Get("code"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "facebook-auth", Value: token.AccessToken})
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(callbackTemplate, token.AccessToken)))
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "facebook-auth", Value: ""})
}

var fields = map[string]interface{}{"fields": "first_name,last_name,email,picture"}

func profileHandler(w http.ResponseWriter, r *http.Request) {
	const profileTemplate = `
	<html>
		<head><title>Profile</title><head>
		<body>
			<p><img src="%s"/></p>
			<p><b>Name:</b> %s %s</p>
			<p><b>Email:</b> %s</p>
		</body>
	</html>`
	c, err := r.Cookie("facebook-auth")
	if err != nil && err != http.ErrNoCookie {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var token = ""
	if c.Value != "" {
		token = c.Value
	} else if h := r.Header.Get("Authentication"); h != "" {
		if h = strings.TrimPrefix(h, "Bearer "); h != "" {
			token = h
		}
	}
	if token == "" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var u struct {
		ID      string `json:"id"`
		Name    string `json:"first_name"`
		Surname string `json:"last_name"`
		Email   string `json:"email"`
		Pic     struct {
			D struct{ URL string } `json:"data"`
		} `json:"picture"`
	}
	if err := facebookCall("GET", "me", token, fields, &u); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, profileTemplate, u.Pic.D.URL, u.Name, u.Surname, u.Email)
}

func facebookCall(method, path, token string, params map[string]interface{}, r interface{}) error {
	const root = "https://graph.facebook.com/v2.10"
	var (
		resp *http.Response
		err  error
	)
	query := url.Values{}
	for k, v := range params {
		query.Set(k, fmt.Sprint(v))
	}
	switch method {
	case "GET":
		query.Set("access_token", token)
		url := fmt.Sprintf("%s/%s?%s", root, path, query.Encode())
		resp, err = http.Get(url)
	case "POST":
		url := fmt.Sprintf("%s/%s?access_token=%s", root, path, token)
		resp, err = http.PostForm(url, query)
	default:
		return fmt.Errorf("Unsupported method %s", method)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if code := resp.StatusCode; code != http.StatusOK && code != http.StatusCreated {
		var v struct{ Error apiError }
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			return fmt.Errorf("Status: %v", code)
		}
		return &v.Error
	}
	return json.NewDecoder(resp.Body).Decode(r)
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

func (a *apiError) Error() string {
	return fmt.Sprintf("%s: %s (%v)", a.Type, a.Message, a.Code)
}
