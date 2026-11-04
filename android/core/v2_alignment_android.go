package main

import("context";"encoding/json")

func(s *Studio) computeRangeAlignment(ctx context.Context,audio,text,language string,duration float64)(alignmentOutput,error){
	request,_:=json.Marshal(map[string]any{"audio":audio,"text":text,"language":language,"ffmpeg":mobilePaths["ffmpeg"]});raw,e:=mobileTask(ctx,"alignment",request);if e!=nil{return alignmentOutput{},e};var result alignmentOutput;e=json.Unmarshal(raw,&result);result.Original=text;return result,e
}

func(s *Studio) missingAlignmentMessage()string{return "نزّل نموذج التوقيت من إعدادات أندرويد أولًا / Download the timing model in Android settings"}
