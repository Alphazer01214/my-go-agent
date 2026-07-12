package config

import "github.com/joho/godotenv"

func LoadDotEnv() {
	godotenv.Load()
}
