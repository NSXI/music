package main

func(s *Studio) completedDownload(id string){if t,ok:=s.task(id);ok&&t.UpgradeFor==""{go s.mobileExport(id)}}
