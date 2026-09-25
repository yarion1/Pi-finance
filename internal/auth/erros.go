package auth

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrCredenciais     = errors.New("e-mail ou senha incorretos")
	ErrCodigo          = errors.New("código incorreto")
	ErrCadastroFechado = errors.New("o cadastro é só por convite")
	ErrConviteInvalido = errors.New("convite inválido ou expirado")
	ErrEmailEmUso      = errors.New("já existe uma conta com este e-mail")
	ErrDadosInvalidos  = errors.New("dados inválidos")
	ErrSessao          = errors.New("sessão inválida ou expirada")
	ErrSegundoFator    = errors.New("confirme o segundo fator")
	ErrReautenticar    = errors.New("confirme a senha para continuar")
	ErrNaoPermitido    = errors.New("operação não permitida")
	ErrPasskey         = errors.New("não foi possível validar a passkey")
	ErrUltimoFator     = errors.New("mantenha pelo menos um segundo fator")
)

// ErrBloqueado indica bloqueio temporário por excesso de tentativas.
type ErrBloqueado struct{ Ate time.Time }

func (e ErrBloqueado) Error() string {
	return fmt.Sprintf("muitas tentativas; tente de novo depois das %s", e.Ate.In(fusoBR).Format("15:04"))
}

var fusoBR = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return l
}()
