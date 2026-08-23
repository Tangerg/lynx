// Package operationsflow owns cross-run usage reporting and write-only user
// feedback. Neither concern participates in Run execution state.
package operationsflow

import(
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/accounting"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)
type Store interface{ListUsageRunRecords(context.Context,string,time.Time)([]accounting.RunRecord,error);SessionExists(context.Context,string)(bool,error);CreateFeedbackRecord(context.Context,string,protocol.FeedbackRequest)error}
type IDs interface{New(string)(string,error)}
type Service struct{store Store;ids IDs;now func()time.Time}
func New(store Store,ids IDs)(*Service,error){if store==nil||ids==nil{return nil,errors.New("operationsflow: store and ids are required")};return &Service{store:store,ids:ids,now:time.Now},nil}
type runFacts struct{Metrics protocol.RunMetrics `json:"metrics"`}
func(s *Service)SessionUsage(ctx context.Context,sessionID string)(*protocol.Usage,error){exists,err:=s.store.SessionExists(ctx,sessionID);if err!=nil{return nil,err};if !exists{return nil,protocol.ErrSessionNotFound};records,err:=s.store.ListUsageRunRecords(ctx,sessionID,time.Time{});if err!=nil{return nil,err};total:=accumulator{};byModel:=map[string]*accumulator{};for _,record:=range records{usage,ok,err:=usageOf(record);if err!=nil{return nil,err};if !ok{continue};total.add(usage.ModelUsage);for model,value:=range usage.ByModel{bucket:=byModel[model];if bucket==nil{bucket=&accumulator{};byModel[model]=bucket};bucket.add(value)};if len(usage.ByModel)==0{bucket:=byModel[record.Model];if bucket==nil{bucket=&accumulator{};byModel[record.Model]=bucket};bucket.add(usage.ModelUsage)}};result:=&protocol.Usage{ModelUsage:total.value()};if len(byModel)>0{result.ByModel=map[string]protocol.ModelUsage{};for key,value:=range byModel{result.ByModel[key]=value.value()}};return result,nil}
func(s *Service)UsageSummary(ctx context.Context,request protocol.UsageSummaryRequest)(*protocol.UsageSummary,error){if request.SinceDays<0{return nil,fmt.Errorf("%w: sinceDays must be non-negative",protocol.ErrInvalidParams)};var since time.Time;if request.SinceDays>0{since=s.now().UTC().AddDate(0,0,-request.SinceDays)};records,err:=s.store.ListUsageRunRecords(ctx,"",since);if err!=nil{return nil,err};total:=accumulator{};providers:=map[string]*bucketAccumulator{};models:=map[string]*bucketAccumulator{};days:=map[string]*bucketAccumulator{};sessions:=map[string]bool{};runs:=0;for _,record:=range records{usage,ok,err:=usageOf(record);if err!=nil{return nil,err};if !ok{continue};total.add(usage.ModelUsage);providersFor(providers,record.Provider).add(usage.ModelUsage);modelsFor(models,record.Provider+"/"+record.Model).add(usage.ModelUsage);daysFor(days,record.FinishedAt.UTC().Format(time.DateOnly)).add(usage.ModelUsage);sessions[record.SessionID]=true;runs++};return &protocol.UsageSummary{Total:total.value(),ByProvider:buckets(providers,false),ByModel:buckets(models,false),ByDay:buckets(days,true),Sessions:len(sessions),Runs:runs},nil}
func(s *Service)Feedback(ctx context.Context,request protocol.FeedbackRequest)error{if request.Rating!=protocol.FeedbackPositive&&request.Rating!=protocol.FeedbackNegative{return fmt.Errorf("%w: rating is invalid",protocol.ErrInvalidParams)};if len(request.Text)>4000{return fmt.Errorf("%w: feedback text is too long",protocol.ErrInvalidParams)};if request.SessionID==""&&request.RunID==""&&request.ItemID==""&&strings.TrimSpace(request.Text)==""{return fmt.Errorf("%w: feedback has no subject or text",protocol.ErrInvalidParams)};id,err:=s.ids.New("fb_");if err!=nil{return err};return s.store.CreateFeedbackRecord(ctx,id,request)}
func usageOf(record accounting.RunRecord)(protocol.Usage,bool,error){var facts runFacts;if err:=json.Unmarshal(record.Body,&facts);err!=nil{return protocol.Usage{},false,err};if facts.Metrics.Usage==nil{return protocol.Usage{},false,nil};return *facts.Metrics.Usage,true,nil}
type accumulator struct{valueFields protocol.ModelUsage;cost float64;hasCost bool}
func(a *accumulator)add(value protocol.ModelUsage){a.valueFields.InputTokens+=value.InputTokens;a.valueFields.OutputTokens+=value.OutputTokens;a.valueFields.CacheReadTokens+=value.CacheReadTokens;a.valueFields.CacheWriteTokens+=value.CacheWriteTokens;a.valueFields.ReasoningTokens+=value.ReasoningTokens;if value.CostUSD!=nil{a.cost+=*value.CostUSD;a.hasCost=true}}
func(a accumulator)value()protocol.ModelUsage{value:=a.valueFields;if a.hasCost{cost:=a.cost;value.CostUSD=&cost};return value}
type bucketAccumulator struct{accumulator;runs int}
func(a *bucketAccumulator)add(value protocol.ModelUsage){a.accumulator.add(value);a.runs++}
func providersFor(values map[string]*bucketAccumulator,key string)*bucketAccumulator{return bucketFor(values,key)}
func modelsFor(values map[string]*bucketAccumulator,key string)*bucketAccumulator{return bucketFor(values,key)}
func daysFor(values map[string]*bucketAccumulator,key string)*bucketAccumulator{return bucketFor(values,key)}
func bucketFor(values map[string]*bucketAccumulator,key string)*bucketAccumulator{value:=values[key];if value==nil{value=&bucketAccumulator{};values[key]=value};return value}
func buckets(values map[string]*bucketAccumulator,keyOrder bool)[]protocol.UsageBucket{result:=make([]protocol.UsageBucket,0,len(values));for key,value:=range values{result=append(result,protocol.UsageBucket{Key:key,ModelUsage:value.value(),Runs:value.runs})};slices.SortFunc(result,func(a,b protocol.UsageBucket)int{if keyOrder{return strings.Compare(a.Key,b.Key)};if a.InputTokens>b.InputTokens{return -1};if a.InputTokens<b.InputTokens{return 1};return strings.Compare(a.Key,b.Key)});return result}
