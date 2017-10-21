package sample

const (
	// sample运行的环境配置。如果您需要运行sample，请先修成您的配置。
	endpoint   string = "<endpoint>"
	accessID   string = "<AccessKeyId>"
	accessKey  string = "<AccessKeySecret>"
	bucketName string = "<my-bucket>"

	// 运行cname的示例程序sample/cname_sample的示例程序的配置。
	// 如果您需要运行sample/cname_sample，请先修成您的配置。
	endpoint4Cname   string = "<endpoint>"
	accessID4Cname   string = "<AccessKeyId>"
	accessKey4Cname  string = "<AccessKeySecret>"
	bucketName4Cname string = "<my-cname-bucket>"

	// 运行sample时的Object名称
	objectKey string = "my-object"

	// 运行sample需要的资源，即sample目录目录下的BingWallpaper-2015-11-07.jpg
	// 和The Go Programming Language.html，请根据实际情况修改
	localFile     string = "src/sample/BingWallpaper-2015-11-07.jpg"
	htmlLocalFile string = "src/sample/The Go Programming Language.html"
	newPicName    string = "src/sample/NewBingWallpaper-2015-11-07.jpg"
)
