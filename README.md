# JakkServ(er)

Just a web server with random-ish functions, such as:

- url shortener (/puturl and /geturl)
- client ip echo (/ip)
- email notification sender (/notify)

## To Use

1. Set values in example.config.ini and rename it to simply `config.ini`

2. `go run main.go`


## Notes

Only /puturl and /notify require auth

multiple emails can be passed to the sendto config value as long as they are comma-separated
