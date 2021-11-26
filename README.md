# independ

This repo contains the source code for the server at [https://independ.org](https://independ.org).

## Requirements

- go 1.16+
- sqlite3

## Config

At startup, the server reads a `config.toml` file in the working directory. Here is an example of the config file:

    [server]
    port = 8080
    
    [database]
    source = "/var/lib/independ/independ.db"

    [mail]
    server = "smtp.example.com"
    username = "me@example.com"
    password = "..."
    error_to = "me@example.com"

    [pages]
    path = "pages"
    buttons = ["About"]

The mail settings are used to email panic stack traces to the `error_to` address. If you don't want or need this, you
can remove the mail section. In that case, the panic stack traces are shown in the browser to the visitor. This may leak
private information.

The pages section can be used to show extra pages in the top menu on the website.

## Run

Start with:

    go run main.go

Open [http://localhost:8080](http://localhost:8080) in your browser.

## License

This repo is available under the MIT license.
