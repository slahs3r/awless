package template

import (
	"fmt"
	"strings"

	"github.com/wallix/awless/template/ast"
)

type Env struct {
	fillers        map[string]interface{}
	externalParams map[string]interface{}

	Resolved         map[string]interface{}
	AliasFunc        func(key, alias string) string
	MissingHolesFunc func(string) interface{}
}

func NewEnv() *Env {
	return &Env{
		Resolved:         make(map[string]interface{}),
		AliasFunc:        func(k, v string) string { return v },
		MissingHolesFunc: func(s string) interface{} { return s },
	}
}

func (e *Env) AddFillers(fills ...map[string]interface{}) {
	if e.fillers == nil {
		e.fillers = make(map[string]interface{})
	}

	for _, f := range fills {
		for k, v := range f {
			e.fillers[k] = v
		}
	}
}

func (e *Env) AddExternalParams(exts ...map[string]interface{}) {
	if e.externalParams == nil {
		e.externalParams = make(map[string]interface{})
	}

	for _, f := range exts {
		for k, v := range f {
			e.externalParams[k] = v
		}
	}
}
func Compile(tpl *Template, env *Env) (*Template, *Env, error) {
	pass := newMultiPass(
		mergeExternalParamsPass,
		resolveHolesPass,
		resolveAliasPass,
		resolveMissingHolesPass,
	)

	return pass.compile(tpl, env)
}

type compileFunc func(*Template, *Env) (*Template, *Env, error)

type multiPass struct {
	passes []compileFunc
}

func newMultiPass(passes ...compileFunc) *multiPass {
	return &multiPass{passes: passes}
}

func (p *multiPass) compile(tpl *Template, env *Env) (newTpl *Template, newEnv *Env, err error) {
	newTpl, newEnv = tpl, env
	for _, pass := range p.passes {
		newTpl, newEnv, err = pass(newTpl, newEnv)
		if err != nil {
			return
		}
	}

	return
}

func resolveHolesPass(tpl *Template, env *Env) (*Template, *Env, error) {
	if env.Resolved == nil {
		env.Resolved = make(map[string]interface{})
	}

	each := func(expr *ast.CommandNode) {
		processed := expr.ProcessHoles(env.fillers)
		for key, v := range processed {
			env.Resolved[expr.Entity+"."+key] = v
		}
	}

	tpl.visitCommandNodes(each)

	return tpl, env, nil
}

func resolveMissingHolesPass(tpl *Template, env *Env) (*Template, *Env, error) {
	uniqueHoles := make(map[string]string)
	tpl.visitCommandNodes(func(cmd *ast.CommandNode) {
		for k, v := range cmd.Holes {
			uniqueHoles[k] = v
		}
	})

	fillers := make(map[string]interface{})
	for _, v := range uniqueHoles {
		actual := env.MissingHolesFunc(v)
		fillers[v] = actual
	}

	tpl.visitCommandNodes(func(expr *ast.CommandNode) {
		expr.ProcessHoles(fillers)
	})

	return tpl, env, nil
}

func resolveAliasPass(tpl *Template, env *Env) (*Template, *Env, error) {
	var unresolved []string
	each := func(cmd *ast.CommandNode) {
		for k, v := range cmd.Params {
			if s, ok := v.(string); ok {
				if strings.HasPrefix(s, "@") {
					alias := strings.TrimPrefix(s, "@")
					actual := env.AliasFunc(k, alias)
					if actual == "" {
						unresolved = append(unresolved, actual)
					} else {
						cmd.Params[k] = actual
						delete(cmd.Holes, k)
					}
				}
			}
		}
	}

	tpl.visitCommandNodes(each)

	if len(unresolved) > 0 {
		return tpl, env, fmt.Errorf("cannot resolve aliases: %v", unresolved)
	}

	return tpl, env, nil
}

func mergeExternalParamsPass(tpl *Template, env *Env) (*Template, *Env, error) {
	each := func(cmd *ast.CommandNode) {
		for k, v := range env.externalParams {
			cmd.Params[k] = v
		}
		if len(env.externalParams) > 0 {
			env.externalParams = make(map[string]interface{})
		}

	}

	tpl.visitCommandNodes(each)

	return tpl, env, nil
}