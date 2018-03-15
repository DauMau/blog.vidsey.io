---
title: "An introduction to Facebook API"
description: "Using Facebook API with Golang"
date: "2018-03-15"
categories:
  - "tutorial"
tags:
  - "facebook"
  - "go"
author: "alex"
draft: true
---

Facebook has a really well documented and complete API. 
You can check the [Login Docs](https://developers.facebook.com/docs/facebook-login) 
or the [Graph API docs](https://developers.facebook.com/docs/graph-api/) 
and try it in the [Graph API Explorer](https://developers.facebook.com/tools/explorer/).
Let's take a look at these two.

#### Login

The login complies the [OAuth 2.0](https://oauth.net/2/) standard, that allows to authenticate users with their facebook account. In short:

- Your app redirects the user to **Facebook Login** that authenticates the user.
- He gets redirected to another endpoint of your application, carrying a **code** in the request.
- You app uses that **code** to exchange it with **Facebook Login** for an actual **Token**.
- The token can be used for authentication in the other services (*f.i. Graph API*).

#### Graph API

This part allows to read and write data to Facebook using a RESTful service. 
It can access to every item available in the social network (which you have permission to read/write) from profiles to videos.

You find out more info at [Graph API documentation](https://developers.facebook.com/docs/graph-api/)

# Sample server

We'll try to build a small server that leverages both **Login** and **Graph API** in Go.

## Authentication

First we will take care of the authentication part. We'll have to do some configuration Facebook-side before writing some code.

### Facebook App

We need a Facebook application in order to create our server. It will be identified by an ID/Secret duplet (that we will use in our configuration) and it will have some permissions over Facebook API services.

In order to do so, we can visit the [Apps dashboard](https://developers.facebook.com/apps/) and create a new App. Here we can take note of **App ID** and **App Secret**, available under the App setting page, we will need them later. 

![Facebook App configuration](/images/facebook_appid.jpg)

The we can add the **Facebook Login** functionality, skip the *quick start* and go directly into its settings.

![Facebook Login configuration](/images/facebook_callback.jpg)

The value that we want to change is **Valid OAuth redirect URIs**, that we will set with `http://localhost:8080/callback` and save changes. This is the value that we'll use for local development, when your app is deployed the url should be in the app domain.

### Login 
Now we need to define a `oath2.Config` struct that allows us to redirect to Facebook website.

{{< code file="facebookapi/facebook_api.go" language="go" lines="1-30">}}

The `loginHandler` function needs to redirect to Facebook, in order to do so we can use the `oauth2.Config.AuthCodeURL` method. 

{{< code file="facebookapi/facebook_api.go" language="go" lines="32-34">}}

The `state` variable passed to this method is returned by Facebook to callback.
In this tutorial is static, in your application could be generated and stored, so that it can be verified it in the callback phase, for a greater security.

The redirect will bring the user to a confirmation screen:

![Facebook continue dialog](/images/facebook_continue.jpg)

### Callback

Clicking **continue** will redirect the user to the **OAuth redirect URI** we specified in [Facebook Configuration](#facebook-configuration).

The callback handler reads the code received from the callback call and exchanges it for the **Authentication token**.
We'll set the token as a cookie, show it on screen, and redirect to the `/profile` page.

{{< code file="facebookapi/facebook_api.go" language="go" lines="36-61">}}

## Graph API reading

The `/profile` handler has to verify the user identity then use the Graph API to recover some information. We can see that `/me` endpoint is used to obtain the profile of the owner of the token we're using.

{{< code file="facebookapi/facebook_api.go" language="go" lines="67-110">}}

The `facebookCall` function is a generic function that handles both `GET` and `POST` call.

We are using it to get the user profile via `me` node, and asking for specific fields and passing a `struct` that can decode its json.

It receives:

- the call `method`
- the desired `path`
- an authentication `token`
- a series of `params` to send (*as query string or as POST form*)
- a struct pointer `r` to unmarshal the response.

# Facebook Package

We recently released the [facebook](https://github.com/DauMau/facebook) package that allows you to easily use the Graph API.

It supports the subset of the Graph functionality we are currently using including:

- User Profile
- Albums
- Video Upload (chunked)

If you want to add one of the missing methods just make a PR, you can use the **[Client.Execute](https://godoc.org/github.com/DauMau/facebook#Client.Execute)**
method that supports also file upload.

The package uses [fasthttp](https://github.com/valyala/fasthttp) instead of the standard `net/html`.
This allows the calls to be faster and use the Garbage collector as least as possible and comes very handy if your server application makes a lot of calls.