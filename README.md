
<p align="center">




</p>

[![Star this repo](https://img.shields.io/badge/⭐-Star%20this%20repo-green)](https://github.com/Yyjccc/chidweb/stargazers)


<p align="center">
 <img src="asset/chidweb.ico" width="255" height="255" alt="chidweb">
<br/>

</p>

<div align="center">

# ✨ Chidweb  ✨


![license](https://img.shields.io/badge/license-MIT-green")
![Static Badge](https://img.shields.io/badge/golang-blue)
![issues](https://img.shields.io/github/issues/Yyjccc/chidweb?color=F48D73)
![release](https://img.shields.io/github/release/Yyjccc/chidweb)




</div>



🚀🚀🚀`chidweb` 是一款反向http隧道工具🎉🎉🎉


项目仅供学习，请勿用于非法用途！



✨目标：不出网机器外连，安全设备流量检测绕过，防火墙规则绕过



该项目不能来获取目标的权限，而是隐藏手段
![](./asset/catcoding.gif)


使用的延迟会比较大，请视情况是否使用



📸示意图: 
![通信示意图](/asset/proc.jpg)

## 🛠️Usage 使用



### Client 客户端



```cmd
usage:
    -cf string
        config file path
    -enable-proxy
        enable proxy
    -i int
        heartbeat packet rate (Unit:s) (default 3)
    -lf string
        log file path (empty for stdout)
    -ll string
        log level (panic|fatal|error|warn|info|debug|trace) (default "info")
    -ln
        disable all logging
    -ob
        use obfuscation (default true)
    -p string
        proxy server urls,split:','
    -s string
        server url
    -t string
        target tcp address,split:','
```




- s: （必需参数）服务端地址
- t: (必需参数) c2或者目标tcp地址 ，可以设置多个，以','分隔
- i: 维持心跳的时间间隔，单位为秒，默认3秒 (以实际情况进行衡量)
- ob: 流量混淆，默认为true开启，使用默认的配置进行流量隐藏，设置为false将会对数据包进行明文传输
- p: 代理服务器,http代理服务器，可以设置多个,以','分隔
- enable-proxy: 是否开启代理,默认为false ,设置代理的时候必须设置为true
- cf : 配置文件路径，允许为空（使用默认的配置）
- lf : 输出的日志文件路径,当为空时候，不会产生日志文件
- ll : 日志级别，取值:(panic|fatal|error|warn|info|debug|trace),默认info
- ln : 是否关闭日志输出，设置为true的时候，控制台不会产生日志输出



example
```shell
   client -s http://172.19.173.38:8080 -t 127.0.0.1:8085,127.0.0.1:7077 -enable-proxy -p http://127.0.0.1:8090
```



### Server 服务端



当机器上有一定执行权限，且有可用端口的时候，可用



```cmd
usage:
    -cf string 
        config file path
    -lf string
        log file path (empty for stdout)
    -ll string
        log level (panic|fatal|error|warn|info|debug|trace) (default "info")
    -ln
        disable all logging
    -p string
        tcp listener port  (default "0")
    -s int
        http listener port (default 8080)
```


- p : 指定监听的端口，默认为0（操作系统随机分配一个端口）
- s : http服务监听的端口，默认8080


example
```shell
   server -p 9999 -s 8077 
```



### 配置文件



默认配置：
```yaml
rate: 3
enableProxy: false
proxies:
  - http://127.0.0.1:8090
log_level: info
log_file: ""
key:
  xor: "UXwoRqMyaRkUxjvKifu2rw=="
  aes: "lY4XTVY+PNCMoFwxjHsWQi0jW0oNqfScVIUk/KE6a3M="
  rc4: "UXwoRqMyaRkUxjvKifu2rw=="
  image: "iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAA60lEQVR4nOzQQQkAIADAQBH7V9YK7iXCXYKxtQe35uuAn5gVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWcAAAA///rKQHKXVy7dAAAAABJRU5ErkJggg=="
custom:
  - name: fake-nacos
    path: /nacos/v2/ns/instance
    description: disguised as a nacos interface traffic
    probability: 50
    request:
      method: PUT
      headers:
        - User-Agent:Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36
        - Content-Type:application/x-www-form-urlencoded
      transformers: rc4->base64->append
      before_append: 'ip=127.0.0.1&weight=0.9&clusterName=DEFAULT&groupName=DEFAULT_GROUP&ephemeral=true&metadata={"driver-class-name":"com.mysql.cj.jdbc.Driver","jwt":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVC1QbHVzIn0.eyJzdW'
      after_append: '.UX78gGcFOZ0r0kAGR59uGQnNPr3qugFqnQKEHnOLbFE","namespace":"public","data":"eyJzdWIiOiUX78gGcFOZ0r0kAGR59uGQnNPr3qugFqnQKEHnOLbFEUzI1NiIsInR5cCIiOTE2MjM5cCI6IkpXVC1cCI6IkpXVC1"}'
    response:
      code: 200
      transformers: aes->base64->append
      before_append: '{"code": 0,"message": "success","data":{"clusterName":"DEFAULT","weight":1.0,"healthy": true,"instanceId": null,"metadata":{"data":"eyJzdWI'
      after_append:  'NPr3qugFqnQ"}}}'
  - name: fake-image
    path: /static/img/common-logo.png
    probability: 50
    request:
      method: POST
      transformers: rc4->image
    response:
      code: 200
      transformers: aes->image
```

#### 整体配置



- <b>rate<b/> : client端发包的时间间隔，单位s
- enableProxy : 是否开启代理
- proxies : http代理
- log_level : 日志级别
- log_file : 输出的日志文件
- key : 密钥配置
    - aes : aes中的密钥，base64编码
    - rc4: rc4算法中的密钥，base64编码
    - xor； 异或计算的密钥，base64编码
    - image: 图片填充的原数据,base64编码
- custom: 是一个配置数组,每一项都是一种数据包处置方式，client按照概率触走对应的配置



#### custom
自定义流量混淆规则
- name(非必需): 规则名称
- path: 服务端请求路径
- description(非必需) :规则描述
- probability: 触发概率(0-100),所有规则概率相加必须要等于100
- request: 客户端处理配置
- response: 服务端处理配置


##### request
- method: 请求方法
- headers: 请求头,键值对以:分隔
- before_append :前面填充的数据，当使用append 的时候必须设置
- after_append :后面填充的数据，当使用append 的时候必须设置
- transformers(必需) :处理链，支持(base64,xor,hex,rc4,aes,append,image),按照处理顺序排序，以->分隔，例如： xor->aes->base64

#### response
- code: 响应码
其他字段同request



⭐ **喜欢这个项目？别忘了点亮 Star 支持哦！** ⭐


![count](https://api.moedog.org/count/@Yyjccc.readme)



![show](https://repobeats.axiom.co/api/embed/af6e99dabd9537151c3144f6ee604565ecf8515a.svg)




![stars](https://starchart.cc/Yyjccc/chidweb.svg)