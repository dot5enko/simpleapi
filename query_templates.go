package simpleapi

import (
	"log"

	"github.com/tidwall/gjson"
)

type PQueryArgProcessor func(args gjson.Result, filters HM) (HM, error)

type QueryTemplate struct {
}

type predefinedQuery struct {
	name          string
	filters       HM
	requiredArgs  []string
	argsProcessor PQueryArgProcessor
	override      ListQueryParams
}

func newPredefinedQuery(name string, filters HM, argsProcessor PQueryArgProcessor, requiredArgs []string, o ListQueryParams) predefinedQuery {

	result := predefinedQuery{
		name:          name,
		filters:       filters,
		argsProcessor: argsProcessor,
		override:      o,
		requiredArgs:  requiredArgs,
	}

	return result

}

func (it *CrudConfig[T, CtxType]) AddQueryTemplate(name string, qparams ListQueryParams, filters HM, argsH PQueryArgProcessor, requiredArgs ...string) *CrudConfig[T, CtxType] {

	it.predefinedQueries[name] = newPredefinedQuery(name, filters, argsH, requiredArgs, qparams)

	return it
}

func (c *CrudConfig[T, CtxType]) ParsePredefinedQuery(qparams ListQueryParams) (filter HM, override ListQueryParams, err *RespErr) {
	if qparams.PredefinedQuery != "" {

		// log.Printf("parsing a template query for %s ", qparams.PredefinedQuery)

		// check if there any predefined queries
		if len(c.predefinedQueries) > 0 {

			// log.Printf("there's some predefined query : %d", len(c.predefinedQueries))

			pq, ok := c.predefinedQueries[qparams.PredefinedQuery]

			if !ok {

				// log.Printf("not found")

				err = NewRespErr(400, HM{
					"msg":  "q not found",
					"q":    qparams.PredefinedQuery,
					"code": "PQ1",
				})
				return
			}

			qArgs := qparams.PredefinedQueryArgs
			var qArgsParsed gjson.Result
			if qArgs != "" {
				qArgsParsed = gjson.Parse(qArgs)
				if !qArgsParsed.Exists() {

					// userAuthData.log_format("unable to parse predefined q args json `%s`", qArgs)

					err = NewRespErr(400, HM{
						"msg":  "malformed args",
						"code": "PQ2",
					})
					return
				}
			}

			if len(pq.requiredArgs) > 0 {
				// validate required args

				for _, requiredArg := range pq.requiredArgs {
					argVal := qArgsParsed.Get(requiredArg)
					if !argVal.Exists() {

						err = NewRespErr(400, HM{
							"msg":  "required q arg not provided",
							"code": "PQ3",
							"arg":  requiredArg,
						})
						return
					}

				}

				// log.Printf(" required args check passed ")
			} else {
				log.Printf("no required args was set ")
			}

			if pq.argsProcessor != nil {

				func() {
					defer func() {
						rec := recover()
						if rec != nil {

							// log to debug logger

							err = NewRespErr(500, HM{
								"msg": "err processing predefined q",
							})
						}
					}()

					var _err error
					filter, _err = pq.argsProcessor(qArgsParsed, pq.filters)

					if _err != nil {
						err = NewRespErr(500, HM{
							"msg": "error processing predefined q args",
						})
					}
				}()
			} else {
				filter = pq.filters

			}

			override = pq.override
			return

		} else {
			// userAuthData.log_format("request has query args, but no predefined queries configured for crud group")

			err = NewRespErr(400, HM{
				"msg":  "wrong q",
				"code": "PQ3",
			})
			return
		}
	}

	err = NewRespErr(500, HM{
		"msg":    "unexpected predefined q",
		"code":   "PQ4",
		"params": qparams,
	})

	return
}
