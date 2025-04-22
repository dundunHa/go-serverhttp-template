package dao

import "database/sql"

type User struct {
	ID   int
	Name string
}

type UserDAO interface {
	FindByID(id int) (*User, error)
}

type userDAO struct{ db *sql.DB }

func NewUserDAO(db *sql.DB) UserDAO {
	return &userDAO{db: db}
}

func (d *userDAO) FindByID(id int) (*User, error) {
	u := &User{}
	err := d.db.QueryRow("SELECT id,name FROM users WHERE id=$1", id).
		Scan(&u.ID, &u.Name)

	return u, err
}
