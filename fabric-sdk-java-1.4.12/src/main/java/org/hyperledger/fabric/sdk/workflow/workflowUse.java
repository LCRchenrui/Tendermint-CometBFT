package org.hyperledger.fabric.sdk.workflow;
import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.security.InvalidKeyException;
import java.security.KeyFactory;
import java.security.NoSuchAlgorithmException;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.SignatureException;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.PKCS8EncodedKeySpec;
import java.util.Base64;

public class workflowUse {
    private static PrivateKey privateKey=null;
    private static String mspId=null;
    private static String certificate=null;
    public static String getCertificate() {
        return certificate;
    }
    public static String getMspId() {
        return mspId;
    }

    public static PrivateKey getPriavteKey() {
        return privateKey;
    }

    public static void initSdkWorkflowUse() throws NoSuchAlgorithmException, InvalidKeySpecException {
        initMspId();
        initCertificate();
        initPrivateKey();
    }

    private static void initCertificate() {
        File mspConfig=new File(System.getenv("CORE_PEER_MSPCONFIGPATH")+"/signcerts");
        String certificatePath=mspConfig.getAbsolutePath()+"/"+mspConfig.list()[0];
        certificate=readFile(certificatePath);
    }

    private static void initMspId() {
        mspId=System.getenv("CORE_PEER_LOCALMSPID");
    }

    //这个是初始化
    private static void initPrivateKey() throws InvalidKeySpecException, NoSuchAlgorithmException {
        File mspConfig=new File(System.getenv("CORE_PEER_MSPCONFIGPATH")+"/keystore");
        String fileContent=readFileWithFilter(mspConfig.getAbsolutePath()+"/"+mspConfig.list()[0]).replace("-----BEGIN PRIVATE KEY-----","")
        .replace("-----END PRIVATE KEY-----", "");
        // String privateKeyString=fileContent.replaceAll("\n", "");
        // PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(Base64.getDecoder().decode(privateKeyString));
        PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(Base64.getDecoder().decode(fileContent));
        privateKey=KeyFactory.getInstance("EC").generatePrivate(keySpec);
    }
//     private static void initPrivateKey() throws InvalidKeySpecException, NoSuchAlgorithmException {
    
//         try {
//             File mspConfig = new File(System.getenv("CORE_PEER_MSPCONFIGPATH") + "/keystore");
//             File privateKeyFile = new File(mspConfig, mspConfig.list()[0]);
            
//             // 读取文件内容
//             BufferedReader reader = new BufferedReader(new FileReader(privateKeyFile));
//             StringBuilder contentBuilder = new StringBuilder();
//             String line;
//             while ((line = reader.readLine()) != null) {
//                 contentBuilder.append(line);
//             }
//             reader.close();
            
//             String fileContent = contentBuilder.toString();
            
//             // 处理PEM格式 - 移除头尾标记和所有空白字符
//             String base64Content = fileContent
//                 .replace("-----BEGIN PRIVATE KEY-----", "")
//                 .replace("-----END PRIVATE KEY-----", "")
//                 .replaceAll("\\s+", ""); // 移除所有空白字符（包括换行符）
            
//             // Base64解码
//             byte[] keyBytes = Base64.getDecoder().decode(base64Content);
//             PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(keyBytes);
//             privateKey = KeyFactory.getInstance("EC").generatePrivate(keySpec);
            
//         } catch (IOException e) {
//             throw new IllegalStateException("Failed to read private key file", e);
//         }
        
// }

    public static byte[] getSinature(byte[] message) throws NoSuchAlgorithmException, InvalidKeySpecException, InvalidKeyException, SignatureException {

        Signature sig=Signature.getInstance("SHA256withECDSA");
        sig.initSign(privateKey);
        sig.update(message);
        sig.verify(message);
        return sig.sign();
    }

    //过滤了换行符
    private static String readFileWithFilter(String path) {
        try {
            //01.FileInputStream（字节流） 实现了InputStream接口，用来读取文件中的字节流，参数是文件或者文件路径+文件名称
            FileInputStream fileInputStream = new FileInputStream(path);
            // 02.将 fileInputStream（字节流） 流作为参数，转为InputStreamReader（字符流）
            InputStreamReader inputStreamReader = new InputStreamReader(fileInputStream, "UTF-8");
            // 03.将 字符流（参数）转为字符串流，带缓冲的流读取，默认缓冲区8k
            BufferedReader bufferedReader = new BufferedReader(inputStreamReader);
            String tempString;
            StringBuilder stringBuilder=new StringBuilder();
            while ((tempString = bufferedReader.readLine()) != null) {// 直接返回读取的字符串
                // 将字符串 添加到 stringBuilder中
                stringBuilder.append(tempString);
            }
            bufferedReader.close();
            inputStreamReader.close();
            fileInputStream.close();
            return stringBuilder.toString(); 
        } catch (IOException e) {
            e.printStackTrace();
            return "error";
        }
    }

        //没有任何处理的读取
        private static String readFile(String path) {
            try {
                //01.FileInputStream（字节流） 实现了InputStream接口，用来读取文件中的字节流，参数是文件或者文件路径+文件名称
                FileInputStream fileInputStream = new FileInputStream(path);
                // 02.将 fileInputStream（字节流） 流作为参数，转为InputStreamReader（字符流）
                InputStreamReader inputStreamReader = new InputStreamReader(fileInputStream, "UTF-8");
                // 03.将 字符流（参数）转为字符串流，带缓冲的流读取，默认缓冲区8k
                BufferedReader bufferedReader = new BufferedReader(inputStreamReader);
                int num=0;
                StringBuilder stringBuilder=new StringBuilder();
                while ((num=bufferedReader.read())!=-1) {
                    stringBuilder.append((char)num);
                }
                bufferedReader.close();
                inputStreamReader.close();
                fileInputStream.close();
                return stringBuilder.toString(); 
            } catch (IOException e) {
                e.printStackTrace();
                return "error";
            }
        }
}
